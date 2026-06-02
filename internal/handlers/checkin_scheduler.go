package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/models"
)

const checkinSchedulerPollInterval = time.Minute

type CheckinSchedulerRunner struct {
	App      *App
	Now      func() time.Time
	SleepFor time.Duration
}

func RunCheckinSchedulerLoop(ctx context.Context, app *App) {
	CheckinSchedulerRunner{App: app, SleepFor: checkinSchedulerPollInterval}.Run(ctx)
}

func (r CheckinSchedulerRunner) Run(ctx context.Context) {
	if r.App == nil || r.App.DB == nil {
		return
	}
	sleepFor := r.SleepFor
	if sleepFor <= 0 {
		sleepFor = checkinSchedulerPollInterval
	}
	ticker := time.NewTicker(sleepFor)
	defer ticker.Stop()

	var lastRunDate string
	for {
		r.RunDue(ctx, &lastRunDate)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r CheckinSchedulerRunner) RunDue(ctx context.Context, lastRunDate *string) bool {
	settings, err := r.App.systemSettings()
	if err != nil {
		log.Printf("自动签到调度: 读取设置失败: %v", err)
		return false
	}
	if !settings.ScheduleEnabled {
		return false
	}
	location, err := time.LoadLocation(strings.TrimSpace(settings.Timezone))
	if err != nil {
		log.Printf("自动签到调度: 时区无效: %v", err)
		return false
	}
	hour, minute, err := parseDailyRunTime(settings.DailyRunTime)
	if err != nil {
		log.Printf("自动签到调度: 执行时间无效: %v", err)
		return false
	}
	now := r.now().In(location)
	dueAt := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, location)
	runDate := now.Format("2006-01-02")
	if now.Before(dueAt) || (lastRunDate != nil && *lastRunDate == runDate) {
		return false
	}
	alreadyRun, err := r.scheduledRunExists(now, location)
	if err != nil {
		log.Printf("自动签到调度: 检查当天执行记录失败: %v", err)
		return false
	}
	if alreadyRun {
		if lastRunDate != nil {
			*lastRunDate = runDate
		}
		return false
	}
	runs, err := r.App.runCheckinBatch(ctx, nil, settings.OnlyEnabledSites, "scheduled", settings)
	if err != nil {
		log.Printf("自动签到调度: 执行失败: %v", err)
		return false
	}
	successCount, failedCount := checkinRunStatusCounts(runs)
	log.Printf("自动签到调度: 已执行一次签到，成功 %d，失败 %d", successCount, failedCount)
	if lastRunDate != nil {
		*lastRunDate = runDate
	}
	return true
}

func (r CheckinSchedulerRunner) scheduledRunExists(now time.Time, location *time.Location) (bool, error) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).UTC()
	end := start.Add(24 * time.Hour)
	var count int64
	err := r.App.DB.Model(&models.CheckinRun{}).
		Where("trigger_type = ? AND started_at >= ? AND started_at < ?", "scheduled", start, end).
		Count(&count).Error
	return count > 0, err
}

func (r CheckinSchedulerRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func parseDailyRunTime(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, strconv.ErrSyntax
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, strconv.ErrSyntax
	}
	return hour, minute, nil
}
