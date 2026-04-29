package handlers

import "net/http"

func (a *App) Plugins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.PluginManager.ListMeta())
}
