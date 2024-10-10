package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/signature"
)

const maxRequestBodySizeBytes = 30 * 1024 * 1024 // 30 MB, @configurable

// Errors returned by the orderflow proxy
// all errors are HTTP 200 jsonrpc responses except some that return proper http error code and a string
var (
	errMethodNotAllowed = "Only POST method is allowded"
	errWrongContentType = "Content-Type must be application/json"
	errMarshalResponse  = "Failed to marshal response"

	// jsonrpc error codes
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeCustomError    = -32000

	errBodySize = JSONRPCError{
		Code:    -32602,
		Message: fmt.Sprintf("Request body too large, max body size %d", maxRequestBodySizeBytes),
	}
	errInvalidRequest = func(err error) *JSONRPCError {
		var anyErr any = err.Error()
		return &JSONRPCError{
			Code:    CodeInvalidRequest,
			Message: "Invalid JSON request",
			Data:    &anyErr,
		}
	}
	errMethodNotFound = JSONRPCError{
		Code:    CodeMethodNotFound,
		Message: "Method not found",
	}
	errSignatureNotFound = JSONRPCError{
		Code:    CodeInvalidRequest,
		Message: "Signature header not set " + signature.HTTPHeader,
	}
	errSignatureNotCorrect = func(err error) *JSONRPCError {
		var anyErr any = err.Error()
		return &JSONRPCError{
			Code:    CodeInvalidRequest,
			Message: "Request signature is not correct",
			Data:    &anyErr,
		}
	}
	errExpectedOneParam        = errors.New("request must have one param element")
	errFailToParseRequestParam = func(err error) *JSONRPCError {
		var anyErr any = err.Error()
		return &JSONRPCError{
			Code:    CodeInvalidParams,
			Message: "Failed to parse request params",
			Data:    &anyErr,
		}
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

func respondWithJSONRPCResponse(w http.ResponseWriter, r *http.Request, response *JSONRPCResponse, log *slog.Logger) {
	w.WriteHeader(http.StatusOK)
	responseBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, errMarshalResponse, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(responseBytes)
	if err != nil {
		log.Warn("Failed to write response", "err", err)
	}
}

func respondWithJSONRPCError(w http.ResponseWriter, r *http.Request, id any, err *JSONRPCError, log *slog.Logger) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  nil,
		Error:   err,
	}
	respondWithJSONRPCResponse(w, r, &resp, log)
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
		respondWithJSONRPCError(w, r, nil, &errBodySize, prx.log)
		return
	}

	var req JSONRPCRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		respondWithJSONRPCError(w, r, nil, errInvalidRequest(err), prx.log)
		return
	}

	// verify signature
	// if public endpoint and its not signed this will be 0x0
	var signer common.Address
	signatureHeader := r.Header.Get(signature.HTTPHeader)
	if signatureHeader == "" {
		if !publicEndpoint {
			respondWithJSONRPCError(w, r, req.ID, &errSignatureNotFound, prx.log)
			return
		}
	} else {
		signer, err = signature.Verify(signatureHeader, body)
		if err != nil {
			respondWithJSONRPCError(w, r, req.ID, errSignatureNotCorrect(err), prx.log)
			return
		}
	}

	jsonrpcErr := prx.HandleRequest(req, signer, publicEndpoint)

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  nil,
		Error:   nil,
	}
	if jsonrpcErr != nil {
		response.Error = jsonrpcErr
	}
	respondWithJSONRPCResponse(w, r, &response, prx.log)
}

type ParsedRequest struct {
	publicEndpoint        bool
	signer                common.Address
	ethSendBundle         *EthSendBundleArgs
	mevSendBundle         *MevSendBundleArgs
	ethCancelBundle       *EthCancelBundleArgs
	ethSendRawTransaction *EthSendRawTransactionArgs
	bidSubsidiseBlock     *BidSubsisideBlockArgs
}

func parseParams[A any](params []json.RawMessage) (*A, *JSONRPCError) {
	if len(params) != 1 {
		return nil, errFailToParseRequestParam(errExpectedOneParam)
	}
	var param A
	err := json.Unmarshal(params[0], &param)
	if err != nil {
		return nil, errFailToParseRequestParam(err)
	}
	return &param, nil
}

func (prx *Proxy) HandleRequest(req JSONRPCRequest, signer common.Address, publicEndpoint bool) *JSONRPCError {
	parsedRequest := ParsedRequest{
		publicEndpoint: publicEndpoint,
		signer:         signer,
	}
	switch req.Method {
	case "eth_sendBundle":
		ethSendBundle, err := parseParams[EthSendBundleArgs](req.Params)
		if err != nil {
			return err
		}
		parsedRequest.ethSendBundle = ethSendBundle
	case "mev_sendBundle":
		mevSendBundle, err := parseParams[MevSendBundleArgs](req.Params)
		if err != nil {
			return err
		}
		parsedRequest.mevSendBundle = mevSendBundle
	case "eth_cancelBundle":
		ethCancelBundle, err := parseParams[EthCancelBundleArgs](req.Params)
		if err != nil {
			return err
		}
		parsedRequest.ethCancelBundle = ethCancelBundle
	case "eth_sendRawTransaction":
		ethSendRawTransaction, err := parseParams[EthSendRawTransactionArgs](req.Params)
		if err != nil {
			return err
		}
		parsedRequest.ethSendRawTransaction = ethSendRawTransaction
	case "bid_subsidiseBlock":
		bidSubsidiseBlock, err := parseParams[BidSubsisideBlockArgs](req.Params)
		if err != nil {
			return err
		}
		parsedRequest.bidSubsidiseBlock = bidSubsidiseBlock
	default:
		return &errMethodNotFound
	}
	return prx.HandleParsedRequest(parsedRequest)
}

func (prx *Proxy) HandleParsedRequest(parsedRequset ParsedRequest) *JSONRPCError {
	return nil
}
