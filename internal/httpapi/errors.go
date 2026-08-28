package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"merch/backend/internal/loyalty"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = message
	writeJSON(w, status, b)
}

func writeErr(w http.ResponseWriter, err error) {
	var le *loyalty.Error
	if errors.As(err, &le) {
		writeError(w, statusFor(le.Code), le.Code, le.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, loyalty.CodeInternal, "Внутренняя ошибка")
}

func statusFor(code string) int {
	switch code {
	case loyalty.CodeUnauthorized:
		return http.StatusUnauthorized
	case loyalty.CodeCustomerNotFound:
		return http.StatusNotFound
	case loyalty.CodeCustomerBlocked, loyalty.CodeStaffForbidden:
		return http.StatusForbidden
	case loyalty.CodeDuplicateReceipt:
		return http.StatusConflict
	case loyalty.CodeInsufficientPoints, loyalty.CodeBelowMinRedeem, loyalty.CodeExceedsShare, loyalty.CodeInvalidRequest:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return loyalty.Err(loyalty.CodeInvalidRequest, "Некорректный JSON")
	}
	return nil
}
