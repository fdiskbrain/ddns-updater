package server

import (
	"encoding/json"
	"net/http"
)

func (h *handlers) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // 添加CORS头支持前后端分离
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	records := h.db.SelectAll()
	now := h.timeNow()

	// 将记录转换为适合API返回的格式
	stats := make([]interface{}, len(records))
	for i, record := range records {
		stat := record.JSON(now)
		stats[i] = stat
	}

	// 返回标准REST响应格式
	response := map[string]interface{}{
		"success":   true,
		"status":    "success",
		"message":   "Records retrieved successfully",
		"data":      stats,
		"count":     len(stats),
		"timestamp": now.Unix(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		httpError(w, http.StatusInternalServerError, "failed generating response: "+err.Error())
	}
}
