package link

import (
	"context"
	"encoding/json"
	"fmt"
)

// Call calls method with req, which json.Marshal takes, and decodes the
// reply into a Res. It is the typed form of Peer.Call.
func Call[Req, Res any](ctx context.Context, p *Peer, method string, req Req) (Res, error) {
	var res Res
	raw, err := p.Call(ctx, method, req)
	if err != nil {
		return res, err
	}
	if len(raw) == 0 {
		return res, nil
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return res, fmt.Errorf("link: %s: cannot decode reply: %w", method, err)
	}
	return res, nil
}

// HandleFunc handles method for the peer, with typed argument and result.
// A request that cannot be decoded answers an error, and so does a result
// that cannot be encoded.
func HandleFunc[Req, Res any](p *Peer, method string, fn func(ctx context.Context, req Req) (Res, error)) {
	p.Handle(method, func(ctx context.Context, args []json.RawMessage) (any, error) {
		var req Req
		if len(args) > 0 && len(args[0]) > 0 {
			if err := json.Unmarshal(args[0], &req); err != nil {
				return nil, fmt.Errorf("link: %s: cannot decode request: %w", method, err)
			}
		}
		return fn(ctx, req)
	})
}
