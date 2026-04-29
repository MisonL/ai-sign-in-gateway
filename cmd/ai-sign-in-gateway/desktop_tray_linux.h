#pragma once

void desktop_tray_init(void);
void desktop_tray_loop(void);
void desktop_tray_quit(void);
void desktop_tray_set_icon(const char *path);
void desktop_tray_set_title(const char *title);
void desktop_tray_set_tooltip(const char *tooltip);
int desktop_tray_add_menu_item(const char *title);
void desktop_tray_add_separator(void);
void desktop_tray_disable_item(int id);
void desktop_tray_set_item_title(int id, const char *title);
void desktop_tray_set_item_tooltip(int id, const char *tooltip);
