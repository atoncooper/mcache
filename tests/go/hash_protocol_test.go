package main

import (
	"math"
	"testing"

	net "github.com/atoncooper/mcache/net"
)

func TestHashEncodeDecode_HSet(t *testing.T) {
	req := &net.HashRequest{Cmd: net.CmdHSet, Key: "myhash", Field: "f1", Value: "v1"}
	payload := net.EncodeHashRequest(req)
	decoded, err := net.DecodeHashRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Key != "myhash" || decoded.Field != "f1" || decoded.Value != "v1" {
		t.Fatalf("unexpected: key=%s field=%s value=%s", decoded.Key, decoded.Field, decoded.Value)
	}
}

func TestHashEncodeDecode_HGetAll(t *testing.T) {
	req := &net.HashRequest{Cmd: net.CmdHGetAll, Key: "myhash"}
	payload := net.EncodeHashRequest(req)
	decoded, err := net.DecodeHashRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Key != "myhash" {
		t.Fatalf("expected myhash, got %s", decoded.Key)
	}
}

func TestHashEncodeDecode_HDel(t *testing.T) {
	req := &net.HashRequest{Cmd: net.CmdHDel, Key: "myhash", Fields: []string{"f1", "f2"}}
	payload := net.EncodeHashRequest(req)
	decoded, err := net.DecodeHashRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Key != "myhash" || len(decoded.Fields) != 2 || decoded.Fields[0] != "f1" {
		t.Fatalf("unexpected: key=%s fields=%v", decoded.Key, decoded.Fields)
	}
}

func TestHashEncodeDecode_HIncrBy(t *testing.T) {
	req := &net.HashRequest{Cmd: net.CmdHIncrBy, Key: "myhash", Field: "counter", DeltaI64: 5}
	payload := net.EncodeHashRequest(req)
	decoded, err := net.DecodeHashRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DeltaI64 != 5 {
		t.Fatalf("expected 5, got %d", decoded.DeltaI64)
	}
}

func TestHashEncodeDecode_HIncrByFloat(t *testing.T) {
	req := &net.HashRequest{Cmd: net.CmdHIncrByFloat, Key: "myhash", Field: "score", DeltaF64: 1.5}
	payload := net.EncodeHashRequest(req)
	decoded, err := net.DecodeHashRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(decoded.DeltaF64-1.5) > 0.001 {
		t.Fatalf("expected 1.5, got %f", decoded.DeltaF64)
	}
}

func TestHashEncodeDecode_HMSet(t *testing.T) {
	req := &net.HashRequest{Cmd: net.CmdHMSet, Key: "myhash", FvPairs: []string{"a", "1", "b", "2"}}
	payload := net.EncodeHashRequest(req)
	decoded, err := net.DecodeHashRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.FvPairs) != 4 || decoded.FvPairs[0] != "a" || decoded.FvPairs[1] != "1" {
		t.Fatalf("unexpected: %v", decoded.FvPairs)
	}
}

func TestHashResponseEncodeDecode_Int(t *testing.T) {
	resp := &net.HashResponse{Cmd: net.CmdHSet, Status: net.StatusOK, IntResult: 1}
	payload := net.EncodeHashResponse(resp)
	decoded, err := net.DecodeHashResponse(payload, net.CmdHSet)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.IntResult != 1 {
		t.Fatalf("expected 1, got %d", decoded.IntResult)
	}
}

func TestHashResponseEncodeDecode_Bool(t *testing.T) {
	resp := &net.HashResponse{Cmd: net.CmdHExists, Status: net.StatusOK, BoolResult: true}
	payload := net.EncodeHashResponse(resp)
	decoded, err := net.DecodeHashResponse(payload, net.CmdHExists)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.BoolResult {
		t.Fatal("expected true")
	}
}

func TestHashResponseEncodeDecode_GetAll(t *testing.T) {
	resp := &net.HashResponse{Cmd: net.CmdHGetAll, Status: net.StatusOK, MapResult: map[string]string{"k1": "v1"}}
	payload := net.EncodeHashResponse(resp)
	decoded, err := net.DecodeHashResponse(payload, net.CmdHGetAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.MapResult) != 1 || decoded.MapResult["k1"] != "v1" {
		t.Fatalf("unexpected: %v", decoded.MapResult)
	}
}

func TestHashResponseError(t *testing.T) {
	resp := &net.HashResponse{Cmd: net.CmdHGet, Status: net.StatusErr, ErrMsg: "key not found"}
	payload := net.EncodeHashResponse(resp)
	decoded, err := net.DecodeHashResponse(payload, net.CmdHGet)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Status != net.StatusErr {
		t.Fatal("expected error status")
	}
}
