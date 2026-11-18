package link

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

type addReq struct {
	A, B int
}

type addRes struct {
	Sum int
}

// pair makes two peers over one in-memory connection.
func pair(t *testing.T) (*Peer, *Peer) {
	t.Helper()
	a, b := net.Pipe()
	pa, pb := NewPeer(a), NewPeer(b)
	t.Cleanup(func() {
		pa.Close()
		pb.Close()
	})
	return pa, pb
}

func TestPeerCall(t *testing.T) {
	pa, pb := pair(t)
	HandleFunc[addReq, addRes](pb, "Add", func(_ context.Context, req addReq) (addRes, error) {
		return addRes{Sum: req.A + req.B}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Call[addReq, addRes](ctx, pa, "Add", addReq{A: 2, B: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sum != 5 {
		t.Errorf("sum %d", res.Sum)
	}
}

func TestPeerUnknownMethod(t *testing.T) {
	pa, pb := pair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pa.Call(ctx, "Nope"); err == nil {
		t.Fatal("no error for an unknown method")
	}
	_ = pb
}

func TestPeerEventHeld(t *testing.T) {
	pa, pb := pair(t)
	// The event comes before its listener.
	if err := pb.Emit("ready", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	got := make(chan int, 1)
	pa.OnEvent("ready", func(p json.RawMessage) {
		var v struct{ N int }
		_ = json.Unmarshal(p, &v)
		got <- v.N
	})
	select {
	case n := <-got:
		if n != 1 {
			t.Errorf("held event %d", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the held event never came")
	}
}

func TestPeerEventSubscribed(t *testing.T) {
	pa, pb := pair(t)
	got := make(chan int, 1)
	pa.OnEvent("tick", func(json.RawMessage) { got <- 1 })
	if err := pb.Emit("tick", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the event never came")
	}
}

func TestPeerClose(t *testing.T) {
	pa, pb := pair(t)
	pa.Close()
	select {
	case <-pb.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("closing one peer did not end the other")
	}
	if err := pb.Err(); err == nil {
		t.Fatal("no error after close")
	}
	ctx := context.Background()
	if _, err := pa.Call(ctx, "Any"); err == nil {
		t.Fatal("a call on a closed peer succeeded")
	}
}

func TestCheckHello(t *testing.T) {
	ok := &Message{T: "hello", Proto: Proto, Token: "abc", Frame: &FrameInfo{File: "f.bin", W: 4, H: 4, Stride: 16}}
	if err := checkHello(ok, "abc"); err != nil {
		t.Fatal(err)
	}
	cases := []*Message{
		{T: "welcome", Proto: Proto, Token: "abc", Frame: &FrameInfo{File: "f.bin"}},
		{T: "hello", Proto: Proto + 1, Token: "abc", Frame: &FrameInfo{File: "f.bin"}},
		{T: "hello", Proto: Proto, Token: "abd", Frame: &FrameInfo{File: "f.bin"}},
		{T: "hello", Proto: Proto, Token: "abc"},
	}
	for _, m := range cases {
		if err := checkHello(m, "abc"); err == nil {
			t.Errorf("%+v accepted", m)
		}
	}
}

func TestSendTooLarge(t *testing.T) {
	pa, _ := pair(t)
	big := make([]byte, MaxMessage)
	if err := pa.Send(&Message{T: "event", Payload: big}); err == nil {
		t.Fatal("a message over MaxMessage was sent")
	}
}

func TestRingFrameInfoRoundTrip(t *testing.T) {
	info := FrameInfo{File: "frames-2.bin", Slots: RingSlots, W: 640, H: 480, Stride: 2560, Scale: 2}
	b, err := json.Marshal(&Message{T: "surface", Frame: &info, Size: &SizeInfo{W: 320, H: 240, Scale: 2}})
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Frame == nil || *got.Frame != info {
		t.Errorf("frame info %+v", got.Frame)
	}
	if got.Size == nil || got.Size.W != 320 || got.Size.Scale != 2 {
		t.Errorf("size %+v", got.Size)
	}
}
