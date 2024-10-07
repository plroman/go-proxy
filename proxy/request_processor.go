package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
)

const maxRequestBodySizeBytes = 10 * 1024 * 1024 // 10 MB, @configurable

var (
	errMethodNotAllowed = "Method not allowed"
	errWrongContentType = "Content-Type must be application/json"
	errBodySize         = fmt.Sprintf("Request body too large, max body size %d", maxRequestBodySizeBytes)
	errInvalidJSON      = "Invalid JSON request"
	errMarshalResponse  = "Failed to marshal response"

	errMethodNotFound = JSONRPCError{
		Code:    -32602,
		Message: "Method not found",
	}
)

type JSONRPCRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      any               `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      any              `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError    `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *any   `json:"data,omitempty"`
}

func (prx *Proxy) ServeProxyRequest(w http.ResponseWriter, r *http.Request, publicEndpoint bool) {
	if r.Method != http.MethodPost {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, errWrongContentType, http.StatusUnsupportedMediaType)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySizeBytes)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, errBodySize, http.StatusRequestEntityTooLarge)
		return
	}

	// verify signature here
	var signer common.Address

	var req JSONRPCRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, errInvalidJSON, http.StatusBadRequest)
		return
	}

	jsonrpcErr := prx.HandleRequest(req, signer, publicEndpoint)

	w.WriteHeader(http.StatusOK)

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  nil,
		Error:   nil,
	}

	if jsonrpcErr != nil {
		response.Error = jsonrpcErr
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, errMarshalResponse, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(responseBytes)
	if err != nil {
		prx.log.Warn("Failed to write response", "err", err)
	}
}

func (prx *Proxy) HandleRequest(req JSONRPCRequest, signer common.Address, publicEndpoint bool) *JSONRPCError {
	return &errMethodNotFound
}
