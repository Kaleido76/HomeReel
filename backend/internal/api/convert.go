package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"homereel/backend/internal/fservice"
)

// handleConvert enqueues format-factory conversions for the selected paths
// (video files → an mp4 copy next to them; directories → a sibling " (MP4)"
// folder with the converted copies of their direct-level videos). params carries
// the operations-panel choice (video codec / CRF / audio / burn); when absent
// the fast-MP4 defaults apply. Per-path validation failures are returned
// alongside the enqueued jobs so the caller can show exactly which selection
// could not be queued.
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths  []string               `json:"paths"`
		Params fservice.ConvertParams `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体格式错误")
		return
	}
	if len(body.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "未选择文件或文件夹")
		return
	}
	jobs, errs := s.fsvc.EnqueueConvert(r.Context(), body.Paths, body.Params)
	if len(jobs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_paths", "所选路径均无法转换："+strings.Join(errMessages(errs), "；"))
		return
	}
	// A nil slice would serialize as JSON null and break array-typed consumers.
	if errs == nil {
		errs = []fservice.OpError{}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"jobs": jobs, "errors": errs})
}

func errMessages(errs []fservice.OpError) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Path+": "+e.Message)
	}
	return out
}

// handleConvertProbe returns per-file stream facts for the selected paths
// (directories expand to their direct-level videos) so the format-factory
// operations panel can guide the user and disable irrelevant options.
func (s *Server) handleConvertProbe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体格式错误")
		return
	}
	if len(body.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "未选择文件或文件夹")
		return
	}
	results := s.fsvc.ProbeConvert(r.Context(), body.Paths)
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
