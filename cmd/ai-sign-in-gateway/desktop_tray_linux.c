//go:build desktop_shell && linux

#include <gtk/gtk.h>
#include <stdlib.h>
#include <string.h>

#include "desktop_tray_linux.h"

extern void goDesktopTrayClicked(int id);

static GtkStatusIcon *tray_icon = NULL;
static GtkWidget *tray_menu = NULL;
static GHashTable *tray_items = NULL;
static int next_tray_id = 1;

typedef struct {
  int id;
  char *value;
} tray_string_update;

static void on_menu_item_activate(GtkMenuItem *item, gpointer user_data) {
  goDesktopTrayClicked(GPOINTER_TO_INT(user_data));
}

static void on_popup_menu(GtkStatusIcon *status_icon, guint button, guint activate_time, gpointer user_data) {
  gtk_menu_popup(GTK_MENU(tray_menu), NULL, NULL, gtk_status_icon_position_menu, status_icon, button, activate_time);
}

static gboolean set_icon_idle(gpointer data) {
  char *path = (char *)data;
  if (tray_icon != NULL && path != NULL) {
    gtk_status_icon_set_from_file(tray_icon, path);
  }
  free(path);
  return G_SOURCE_REMOVE;
}

static gboolean set_title_idle(gpointer data) {
  char *title = (char *)data;
  if (tray_icon != NULL && title != NULL) {
    gtk_status_icon_set_title(tray_icon, title);
  }
  free(title);
  return G_SOURCE_REMOVE;
}

static gboolean set_tooltip_idle(gpointer data) {
  char *tooltip = (char *)data;
  if (tray_icon != NULL && tooltip != NULL) {
    gtk_status_icon_set_tooltip_text(tray_icon, tooltip);
  }
  free(tooltip);
  return G_SOURCE_REMOVE;
}

static gboolean disable_item_idle(gpointer data) {
  int id = GPOINTER_TO_INT(data);
  GtkWidget *item = GTK_WIDGET(g_hash_table_lookup(tray_items, GINT_TO_POINTER(id)));
  if (item != NULL) {
    gtk_widget_set_sensitive(item, FALSE);
  }
  return G_SOURCE_REMOVE;
}

static gboolean set_item_title_idle(gpointer data) {
  tray_string_update *update = (tray_string_update *)data;
  GtkWidget *item = GTK_WIDGET(g_hash_table_lookup(tray_items, GINT_TO_POINTER(update->id)));
  if (item != NULL && update->value != NULL) {
    gtk_menu_item_set_label(GTK_MENU_ITEM(item), update->value);
  }
  free(update->value);
  free(update);
  return G_SOURCE_REMOVE;
}

void desktop_tray_init(void) {
  gtk_init(0, NULL);
  tray_items = g_hash_table_new(g_direct_hash, g_direct_equal);
  tray_menu = gtk_menu_new();
  tray_icon = gtk_status_icon_new();
  gtk_status_icon_set_visible(tray_icon, TRUE);
  g_signal_connect(G_OBJECT(tray_icon), "popup-menu", G_CALLBACK(on_popup_menu), NULL);
}

void desktop_tray_loop(void) {
  gtk_main();
}

void desktop_tray_quit(void) {
  g_idle_add((GSourceFunc)gtk_main_quit, NULL);
}

void desktop_tray_set_icon(const char *path) {
  g_idle_add(set_icon_idle, g_strdup(path));
}

void desktop_tray_set_title(const char *title) {
  g_idle_add(set_title_idle, g_strdup(title));
}

void desktop_tray_set_tooltip(const char *tooltip) {
  g_idle_add(set_tooltip_idle, g_strdup(tooltip));
}

int desktop_tray_add_menu_item(const char *title) {
  int id = next_tray_id++;
  GtkWidget *item = gtk_menu_item_new_with_label(title);
  g_hash_table_insert(tray_items, GINT_TO_POINTER(id), item);
  g_signal_connect(G_OBJECT(item), "activate", G_CALLBACK(on_menu_item_activate), GINT_TO_POINTER(id));
  gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), item);
  gtk_widget_show(item);
  return id;
}

void desktop_tray_add_separator(void) {
  GtkWidget *item = gtk_separator_menu_item_new();
  gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), item);
  gtk_widget_show(item);
}

void desktop_tray_disable_item(int id) {
  g_idle_add(disable_item_idle, GINT_TO_POINTER(id));
}

void desktop_tray_set_item_title(int id, const char *title) {
  tray_string_update *update = (tray_string_update *)calloc(1, sizeof(tray_string_update));
  update->id = id;
  update->value = g_strdup(title);
  g_idle_add(set_item_title_idle, update);
}

void desktop_tray_set_item_tooltip(int id, const char *tooltip) {
  (void)id;
  (void)tooltip;
}
