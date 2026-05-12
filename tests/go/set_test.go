package main

import (
	"testing"

	mnet "github.com/atoncooper/mcache/net"
)

// TestSetProtocol tests the encode/decode round-trip for set commands.
func TestSetRequestEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		req  mnet.SetRequest
	}{
		{"SAdd", mnet.SetRequest{Cmd: mnet.CmdSAdd, Key: "myset", Elems: []string{"hello"}}},
		{"SRem", mnet.SetRequest{Cmd: mnet.CmdSRem, Key: "myset", Elems: []string{"hello"}}},
		{"SIsMember", mnet.SetRequest{Cmd: mnet.CmdSIsMember, Key: "myset", Elems: []string{"hello"}}},
		{"SMembers", mnet.SetRequest{Cmd: mnet.CmdSMembers, Key: "myset"}},
		{"SCard", mnet.SetRequest{Cmd: mnet.CmdSCard, Key: "myset"}},
		{"SPop", mnet.SetRequest{Cmd: mnet.CmdSPop, Key: "myset"}},
		{"SRandMember", mnet.SetRequest{Cmd: mnet.CmdSRandMember, Key: "myset", Count: 3}},
		{"SUnion", mnet.SetRequest{Cmd: mnet.CmdSUnion, Keys: []string{"k1", "k2"}}},
		{"SInter", mnet.SetRequest{Cmd: mnet.CmdSInter, Keys: []string{"k1", "k2", "k3"}}},
		{"SDiff", mnet.SetRequest{Cmd: mnet.CmdSDiff, Keys: []string{"k1", "k2"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := mnet.EncodeSetRequest(&tt.req)
			if payload == nil {
				t.Fatal("EncodeSetRequest returned nil")
			}
			decoded, err := mnet.DecodeSetRequest(payload)
			if err != nil {
				t.Fatalf("DecodeSetRequest failed: %v", err)
			}
			if decoded.Cmd != tt.req.Cmd {
				t.Errorf("cmd mismatch: %d != %d", decoded.Cmd, tt.req.Cmd)
			}
			if decoded.Key != tt.req.Key {
				t.Errorf("key mismatch: %q != %q", decoded.Key, tt.req.Key)
			}
			if len(tt.req.Elems) > 0 && len(decoded.Elems) > 0 && decoded.Elems[0] != tt.req.Elems[0] {
				t.Errorf("elem mismatch: %q != %q", decoded.Elems[0], tt.req.Elems[0])
			}
			if len(tt.req.Keys) != len(decoded.Keys) {
				t.Errorf("keys len mismatch: %d != %d", len(tt.req.Keys), len(decoded.Keys))
			}
		})
	}
}

func TestSetResponseEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		resp mnet.SetResponse
	}{
		{"SAdd", mnet.SetResponse{Cmd: mnet.CmdSAdd, Status: mnet.StatusOK, Changed: 1}},
		{"SRem", mnet.SetResponse{Cmd: mnet.CmdSRem, Status: mnet.StatusOK, Changed: 0}},
		{"SIsMember_true", mnet.SetResponse{Cmd: mnet.CmdSIsMember, Status: mnet.StatusOK, IsMember: true}},
		{"SIsMember_false", mnet.SetResponse{Cmd: mnet.CmdSIsMember, Status: mnet.StatusOK, IsMember: false}},
		{"SCard", mnet.SetResponse{Cmd: mnet.CmdSCard, Status: mnet.StatusOK, Card: 42}},
		{"SPop", mnet.SetResponse{Cmd: mnet.CmdSPop, Status: mnet.StatusOK, Elems: []string{"popped"}}},
		{"SMembers", mnet.SetResponse{Cmd: mnet.CmdSMembers, Status: mnet.StatusOK, Elems: []string{"a", "b", "c"}}},
		{"SRandMember", mnet.SetResponse{Cmd: mnet.CmdSRandMember, Status: mnet.StatusOK, Elems: []string{"r", "s"}}},
		{"SUnion", mnet.SetResponse{Cmd: mnet.CmdSUnion, Status: mnet.StatusOK, Elems: []string{"x", "y", "z"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := mnet.EncodeSetResponse(&tt.resp)
			if payload == nil {
				t.Fatal("EncodeSetResponse returned nil")
			}
			decoded, err := mnet.DecodeSetResponse(payload, tt.resp.Cmd)
			if err != nil {
				t.Fatalf("DecodeSetResponse failed: %v", err)
			}
			if decoded.Status != tt.resp.Status {
				t.Errorf("status mismatch: %d != %d", decoded.Status, tt.resp.Status)
			}
			if tt.resp.Cmd == mnet.CmdSAdd || tt.resp.Cmd == mnet.CmdSRem {
				if decoded.Changed != tt.resp.Changed {
					t.Errorf("changed mismatch: %d != %d", decoded.Changed, tt.resp.Changed)
				}
			}
			if tt.resp.Cmd == mnet.CmdSIsMember {
				if decoded.IsMember != tt.resp.IsMember {
					t.Errorf("isMember mismatch")
				}
			}
			if tt.resp.Cmd == mnet.CmdSCard {
				if decoded.Card != tt.resp.Card {
					t.Errorf("card mismatch: %d != %d", decoded.Card, tt.resp.Card)
				}
			}
			if len(tt.resp.Elems) != len(decoded.Elems) {
				t.Errorf("elems len mismatch: %d != %d", len(tt.resp.Elems), len(decoded.Elems))
			}
		})
	}
}
