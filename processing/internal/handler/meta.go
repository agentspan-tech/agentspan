package handler

import (
	"net/http"
)

// NewMetaHandler returns an http.HandlerFunc serving deployment metadata.
// Publicly accessible — exposes only the APP_VERSION string for the UI footer.
func NewMetaHandler(appVersion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{
			"version": appVersion,
		})
	}
}
