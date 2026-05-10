package main

import (
	"testing"

	net "github.com/atoncooper/mcache/net"
)

func TestListEncodeDecode_LPush(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdLPush, Key: "mylist", Elements: []string{"a", "b"}}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Key != "mylist" || len(decoded.Elements) != 2 || decoded.Elements[0] != "a" {
		t.Fatalf("unexpected: key=%s elems=%v", decoded.Key, decoded.Elements)
	}
}

func TestListEncodeDecode_LPop(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdLPop, Key: "mylist"}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Key != "mylist" {
		t.Fatalf("expected mylist, got %s", decoded.Key)
	}
}

func TestListEncodeDecode_LRange(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdLRange, Key: "mylist", Start: 0, Stop: -1}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Start != 0 || decoded.Stop != -1 {
		t.Fatalf("unexpected: start=%d stop=%d", decoded.Start, decoded.Stop)
	}
}

func TestListEncodeDecode_LSet(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdLSet, Key: "mylist", Index: 1, Value: "x"}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Index != 1 || decoded.Value != "x" {
		t.Fatalf("unexpected: index=%d value=%s", decoded.Index, decoded.Value)
	}
}

func TestListEncodeDecode_LInsert(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdLInsert, Key: "mylist", Before: true, Pivot: "b", Value: "x"}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Before || decoded.Pivot != "b" || decoded.Value != "x" {
		t.Fatalf("unexpected: before=%v pivot=%s value=%s", decoded.Before, decoded.Pivot, decoded.Value)
	}
}

func TestListEncodeDecode_BLPop(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdBLPop, Key: "mylist", Timeout: 5000}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Timeout != 5000 {
		t.Fatalf("expected 5000, got %d", decoded.Timeout)
	}
}

func TestListEncodeDecode_LPos(t *testing.T) {
	req := &net.ListRequest{Cmd: net.CmdLPos, Key: "mylist", Value: "a", Rank: 1, Count: 0, MaxLen: 10}
	payload := net.EncodeListRequest(req)
	decoded, err := net.DecodeListRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Rank != 1 || decoded.Count != 0 || decoded.MaxLen != 10 {
		t.Fatalf("unexpected: rank=%d count=%d maxlen=%d", decoded.Rank, decoded.Count, decoded.MaxLen)
	}
}

func TestListResponseEncodeDecode_Int(t *testing.T) {
	resp := &net.ListResponse{Cmd: net.CmdLPush, Status: net.StatusOK, IntResult: 3}
	payload := net.EncodeListResponse(resp)
	decoded, err := net.DecodeListResponse(payload, net.CmdLPush)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.IntResult != 3 {
		t.Fatalf("expected 3, got %d", decoded.IntResult)
	}
}

func TestListResponseEncodeDecode_Str(t *testing.T) {
	resp := &net.ListResponse{Cmd: net.CmdLPop, Status: net.StatusOK, StrResult: "hello"}
	payload := net.EncodeListResponse(resp)
	decoded, err := net.DecodeListResponse(payload, net.CmdLPop)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.StrResult != "hello" {
		t.Fatalf("expected hello, got %s", decoded.StrResult)
	}
}

func TestListResponseEncodeDecode_LRange(t *testing.T) {
	resp := &net.ListResponse{Cmd: net.CmdLRange, Status: net.StatusOK, SliceResult: []string{"a", "b", "c"}}
	payload := net.EncodeListResponse(resp)
	decoded, err := net.DecodeListResponse(payload, net.CmdLRange)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.SliceResult) != 3 || decoded.SliceResult[0] != "a" {
		t.Fatalf("unexpected: %v", decoded.SliceResult)
	}
}

func TestListResponseEncodeDecode_LPos(t *testing.T) {
	resp := &net.ListResponse{Cmd: net.CmdLPos, Status: net.StatusOK, PosResult: []int64{0, 2, 5}}
	payload := net.EncodeListResponse(resp)
	decoded, err := net.DecodeListResponse(payload, net.CmdLPos)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.PosResult) != 3 || decoded.PosResult[1] != 2 {
		t.Fatalf("unexpected: %v", decoded.PosResult)
	}
}

func TestListResponseError(t *testing.T) {
	resp := &net.ListResponse{Cmd: net.CmdLPop, Status: net.StatusErr, ErrMsg: "key not found"}
	payload := net.EncodeListResponse(resp)
	decoded, err := net.DecodeListResponse(payload, net.CmdLPop)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Status != net.StatusErr {
		t.Fatal("expected error status")
	}
}
