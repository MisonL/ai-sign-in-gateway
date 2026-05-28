package database

import (
	"testing"

	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeAdminUsersPromotesFirstAdminOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:normalize-admin-users?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AdminUser{}); err != nil {
		t.Fatal(err)
	}
	admins := []models.AdminUser{
		{Username: "first", PasswordHash: "hash", Role: "", IsEnabled: true},
		{Username: "second", PasswordHash: "hash", Role: "", IsEnabled: true},
	}
	if err := db.Create(&admins).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AdminUser{}).Where("username = ?", "second").Update("is_enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	if err := NormalizeAdminUsers(db); err != nil {
		t.Fatal(err)
	}
	var got []models.AdminUser
	if err := db.Order("id asc").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got[0].Role != models.AdminRoleSuper || !got[0].IsEnabled {
		t.Fatalf("first admin role/enabled = %q/%v", got[0].Role, got[0].IsEnabled)
	}
	if got[1].Role != models.AdminRoleAdmin || got[1].IsEnabled {
		t.Fatalf("second admin role/enabled = %q/%v", got[1].Role, got[1].IsEnabled)
	}
}
