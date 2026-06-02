package handlers

import (
	"strings"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/services"
	"gorm.io/gorm"
)

type duplicateSiteMergeResult struct {
	MergedGroupCount    int    `json:"merged_group_count"`
	DeletedSiteCount    int    `json:"deleted_site_count"`
	RemainingGroupCount int    `json:"remaining_group_count"`
	KeptSiteIDs         []uint `json:"kept_site_ids"`
	DeletedSiteIDs      []uint `json:"deleted_site_ids"`
}

func mergeDuplicateSites(db *gorm.DB) (duplicateSiteMergeResult, error) {
	var result duplicateSiteMergeResult
	groups, err := duplicateSiteGroups(db)
	if err != nil {
		return result, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, group := range groups {
			if err := mergeDuplicateSiteGroup(tx, group, &result); err != nil {
				return err
			}
		}
		if result.MergedGroupCount == 0 {
			return nil
		}
		_, err := services.SyncGatewayRoutes(tx)
		return err
	})
	if err != nil {
		return result, err
	}
	remaining, err := duplicateSiteGroups(db)
	if err != nil {
		return result, err
	}
	result.RemainingGroupCount = len(remaining)
	return result, nil
}

func mergeDuplicateSiteGroup(tx *gorm.DB, group duplicateSiteGroup, result *duplicateSiteMergeResult) error {
	var keep models.Site
	if err := tx.First(&keep, group.SuggestedKeepID).Error; err != nil {
		return err
	}
	var removed []models.Site
	if err := tx.Where("id IN ? AND id <> ?", group.SiteIDs, group.SuggestedKeepID).Find(&removed).Error; err != nil {
		return err
	}
	removedIDs := make([]uint, 0, len(removed))
	for _, site := range removed {
		mergeDuplicateSiteData(&keep, site)
		removedIDs = append(removedIDs, site.ID)
	}
	if len(removed) == 0 {
		return nil
	}
	if err := tx.Save(&keep).Error; err != nil {
		return err
	}
	if err := deleteDuplicateSiteRecords(tx, removedIDs); err != nil {
		return err
	}
	result.KeptSiteIDs = append(result.KeptSiteIDs, keep.ID)
	result.DeletedSiteIDs = append(result.DeletedSiteIDs, removedIDs...)
	result.MergedGroupCount++
	result.DeletedSiteCount += len(removed)
	return nil
}

func mergeDuplicateSiteData(keep *models.Site, duplicate models.Site) {
	keep.Credentials = mergeMissingJSONFields(keep.Credentials, duplicate.Credentials)
	keep.PluginConfig = mergeMissingJSONFields(keep.PluginConfig, duplicate.PluginConfig)
	keep.GroupName = strings.Join(uniqueGroupNames(append(parseGroupNamesGo(keep.GroupName), parseGroupNamesGo(duplicate.GroupName)...)), ",")
	keep.Notes = mergeDuplicateNotes(keep.Notes, duplicate.Notes)
	if !keep.IsEnabled && duplicate.IsEnabled {
		keep.IsEnabled = true
	}
}

func deleteDuplicateSiteRecords(tx *gorm.DB, siteIDs []uint) error {
	siteIDs = uniqueUintIDs(siteIDs)
	if len(siteIDs) == 0 {
		return nil
	}
	var routeIDs []uint
	if err := tx.Model(&models.GatewayRouteState{}).Where("site_id IN ?", siteIDs).Pluck("id", &routeIDs).Error; err != nil {
		return err
	}
	if len(routeIDs) > 0 {
		if err := tx.Where("route_state_id IN ?", routeIDs).Delete(&models.GatewayRouteGroupMember{}).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("site_id IN ?", siteIDs).Delete(&models.GatewayRouteState{}).Error; err != nil {
		return err
	}
	if err := tx.Where("site_id IN ?", siteIDs).Delete(&models.SiteQueueTask{}).Error; err != nil {
		return err
	}
	if err := clearDuplicateSiteReferences(tx, siteIDs); err != nil {
		return err
	}
	return tx.Where("id IN ?", siteIDs).Delete(&models.Site{}).Error
}

func clearDuplicateSiteReferences(tx *gorm.DB, siteIDs []uint) error {
	updates := []func() error{
		func() error {
			return tx.Model(&models.CheckinRun{}).
				Where("site_id IN ?", siteIDs).
				Update("site_id", nil).Error
		},
		func() error {
			return tx.Model(&models.GatewayRequestLog{}).
				Where("site_id IN ?", siteIDs).
				Update("site_id", nil).Error
		},
		func() error {
			return tx.Model(&models.ChatSession{}).
				Where("site_id IN ?", siteIDs).
				Update("site_id", nil).Error
		},
	}
	for _, update := range updates {
		if err := update(); err != nil {
			return err
		}
	}
	return nil
}

func mergeMissingJSONFields(base models.JSONMap, updates models.JSONMap) models.JSONMap {
	out := cloneJSONMap(nonNilJSON(base))
	for key, value := range nonNilJSON(updates) {
		if key == "api_keys" {
			out[key] = mergeAPIKeyLists(out[key], value)
			continue
		}
		if jsonValueIsEmpty(out[key]) && !jsonValueIsEmpty(value) {
			out[key] = value
		}
	}
	return out
}

func uniqueUintIDs(values []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
