package handlers

import (
	"strings"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/services"
	"gorm.io/gorm"
)

const duplicateMergeReferenceBatchSize = 300

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
	if err := deleteDuplicateSiteRecords(tx, keep.ID, removedIDs); err != nil {
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

func deleteDuplicateSiteRecords(tx *gorm.DB, keepID uint, siteIDs []uint) error {
	siteIDs = uniqueUintIDs(siteIDs)
	if keepID == 0 || len(siteIDs) == 0 {
		return nil
	}
	var routeIDs []uint
	if err := tx.Model(&models.GatewayRouteState{}).Where("site_id IN ?", siteIDs).Pluck("id", &routeIDs).Error; err != nil {
		return err
	}
	routeIDMap, err := duplicateRouteIDMap(tx, keepID, siteIDs)
	if err != nil {
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
	if err := reassignDuplicateSiteReferences(tx, keepID, siteIDs, routeIDs, routeIDMap); err != nil {
		return err
	}
	return tx.Where("id IN ?", siteIDs).Delete(&models.Site{}).Error
}

func duplicateRouteIDMap(tx *gorm.DB, keepID uint, siteIDs []uint) (map[uint]uint, error) {
	var removed []models.GatewayRouteState
	if err := tx.Where("site_id IN ?", siteIDs).Find(&removed).Error; err != nil {
		return nil, err
	}
	if len(removed) == 0 {
		return map[uint]uint{}, nil
	}
	var kept []models.GatewayRouteState
	if err := tx.Where("site_id = ?", keepID).Find(&kept).Error; err != nil {
		return nil, err
	}
	keptByRouteSignature := map[string]uint{}
	for _, route := range kept {
		signature := duplicateRouteSignature(route)
		if signature == "" || keptByRouteSignature[signature] != 0 {
			continue
		}
		keptByRouteSignature[signature] = route.ID
	}
	out := map[uint]uint{}
	for _, route := range removed {
		if targetID := keptByRouteSignature[duplicateRouteSignature(route)]; targetID != 0 {
			out[route.ID] = targetID
		}
	}
	return out, nil
}

func duplicateRouteSignature(route models.GatewayRouteState) string {
	fingerprint := strings.TrimSpace(route.KeyFingerprint)
	if fingerprint == "" {
		return ""
	}
	return strings.Join([]string{
		fingerprint,
		normalizeGatewayRouteType(route.RouteType),
		services.NormalizeGatewayRoutePath(route.RoutePath),
	}, "\x00")
}

func reassignDuplicateSiteReferences(tx *gorm.DB, keepID uint, siteIDs []uint, removedRouteIDs []uint, routeIDMap map[uint]uint) error {
	if err := reassignDuplicateRouteLogReferences(tx, routeIDMap); err != nil {
		return err
	}
	if err := clearUnmappedDuplicateRouteReferences(tx, removedRouteIDs, routeIDMap); err != nil {
		return err
	}
	updates := []func() error{
		func() error {
			return updateCheckinRunsInBatches(tx, "site_id IN ?", []any{siteIDs}, "site_id", keepID)
		},
		func() error {
			return updateGatewayRequestLogsInBatches(tx, "site_id IN ?", []any{siteIDs}, "site_id", keepID)
		},
		func() error {
			return updateChatSessionsInBatches(tx, "site_id IN ?", []any{siteIDs}, "site_id", keepID)
		},
	}
	for _, update := range updates {
		if err := update(); err != nil {
			return err
		}
	}
	return nil
}

func reassignDuplicateRouteLogReferences(tx *gorm.DB, routeIDMap map[uint]uint) error {
	removedIDs := sortedUintKeys(routeIDMap)
	if len(removedIDs) == 0 {
		return nil
	}
	for start := 0; start < len(removedIDs); start += duplicateMergeReferenceBatchSize {
		end := min(start+duplicateMergeReferenceBatchSize, len(removedIDs))
		routeIDChunk := removedIDs[start:end]
		routeIDChunkMap := uintMapSubset(routeIDMap, routeIDChunk)
		if err := reassignDuplicateRouteLogReferenceChunk(tx, routeIDChunk, routeIDChunkMap); err != nil {
			return err
		}
	}
	return nil
}

func reassignDuplicateRouteLogReferenceChunk(tx *gorm.DB, routeIDs []uint, routeIDMap map[uint]uint) error {
	var lastID uint
	for {
		ids, err := nextGatewayRequestLogIDs(tx, "route_state_id IN ?", []any{routeIDs}, lastID)
		if err != nil || len(ids) == 0 {
			return err
		}
		if err := updateGatewayRequestLogRouteStateBatch(tx, ids, routeIDMap); err != nil {
			return err
		}
		lastID = ids[len(ids)-1]
	}
}

func updateGatewayRequestLogRouteStateBatch(tx *gorm.DB, logIDs []uint, routeIDMap map[uint]uint) error {
	caseParts := make([]string, 0, len(routeIDMap))
	args := make([]any, 0, len(routeIDMap)*2)
	for _, removedID := range sortedUintKeys(routeIDMap) {
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, removedID, routeIDMap[removedID])
	}
	statement := "CASE route_state_id " + strings.Join(caseParts, " ") + " ELSE route_state_id END"
	return tx.Model(&models.GatewayRequestLog{}).
		Where("id IN ?", logIDs).
		Update("route_state_id", gorm.Expr(statement, args...)).Error
}

func sortedUintKeys(values map[uint]uint) []uint {
	keys := make([]uint, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		value := keys[i]
		j := i - 1
		for j >= 0 && keys[j] > value {
			keys[j+1] = keys[j]
			j--
		}
		keys[j+1] = value
	}
	return keys
}

func uintMapSubset(values map[uint]uint, keys []uint) map[uint]uint {
	out := make(map[uint]uint, len(keys))
	for _, key := range keys {
		if value := values[key]; value != 0 {
			out[key] = value
		}
	}
	return out
}

func clearUnmappedDuplicateRouteReferences(tx *gorm.DB, removedRouteIDs []uint, routeIDMap map[uint]uint) error {
	unmapped := make([]uint, 0, len(removedRouteIDs))
	for _, routeID := range removedRouteIDs {
		if routeIDMap[routeID] == 0 {
			unmapped = append(unmapped, routeID)
		}
	}
	if len(unmapped) == 0 {
		return nil
	}
	for start := 0; start < len(unmapped); start += duplicateMergeReferenceBatchSize {
		end := min(start+duplicateMergeReferenceBatchSize, len(unmapped))
		if err := updateGatewayRequestLogsInBatches(tx, "route_state_id IN ?", []any{unmapped[start:end]}, "route_state_id", nil); err != nil {
			return err
		}
	}
	return nil
}

func updateCheckinRunsInBatches(tx *gorm.DB, where string, args []any, column string, value any) error {
	var lastID uint
	for {
		ids, err := nextCheckinRunIDs(tx, where, args, lastID)
		if err != nil || len(ids) == 0 {
			return err
		}
		if err := tx.Model(&models.CheckinRun{}).Where("id IN ?", ids).Update(column, value).Error; err != nil {
			return err
		}
		lastID = ids[len(ids)-1]
	}
}

func updateGatewayRequestLogsInBatches(tx *gorm.DB, where string, args []any, column string, value any) error {
	var lastID uint
	for {
		ids, err := nextGatewayRequestLogIDs(tx, where, args, lastID)
		if err != nil || len(ids) == 0 {
			return err
		}
		if err := tx.Model(&models.GatewayRequestLog{}).Where("id IN ?", ids).Update(column, value).Error; err != nil {
			return err
		}
		lastID = ids[len(ids)-1]
	}
}

func updateChatSessionsInBatches(tx *gorm.DB, where string, args []any, column string, value any) error {
	var lastID uint
	for {
		ids, err := nextChatSessionIDs(tx, where, args, lastID)
		if err != nil || len(ids) == 0 {
			return err
		}
		if err := tx.Model(&models.ChatSession{}).Where("id IN ?", ids).Update(column, value).Error; err != nil {
			return err
		}
		lastID = ids[len(ids)-1]
	}
}

func nextCheckinRunIDs(tx *gorm.DB, where string, args []any, afterID uint) ([]uint, error) {
	var ids []uint
	err := tx.Model(&models.CheckinRun{}).
		Where("id > ?", afterID).
		Where(where, args...).
		Order("id asc").
		Limit(duplicateMergeReferenceBatchSize).
		Pluck("id", &ids).Error
	return ids, err
}

func nextGatewayRequestLogIDs(tx *gorm.DB, where string, args []any, afterID uint) ([]uint, error) {
	var ids []uint
	err := tx.Model(&models.GatewayRequestLog{}).
		Where("id > ?", afterID).
		Where(where, args...).
		Order("id asc").
		Limit(duplicateMergeReferenceBatchSize).
		Pluck("id", &ids).Error
	return ids, err
}

func nextChatSessionIDs(tx *gorm.DB, where string, args []any, afterID uint) ([]uint, error) {
	var ids []uint
	err := tx.Model(&models.ChatSession{}).
		Where("id > ?", afterID).
		Where(where, args...).
		Order("id asc").
		Limit(duplicateMergeReferenceBatchSize).
		Pluck("id", &ids).Error
	return ids, err
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
