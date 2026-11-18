module lightcode.ai/client

go 1.27.3

// The assistant as its own project: it draws its own native UI (MyGo's ui),
// embeds itself in the editor's window through ada_link, and owns the
// network call. It imports no editor package.
require (
	github.com/egoist/mygo v0.7.4
	github.com/learningdog1/ada_link v0.0.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/egoist/mygo => ../../mygo

replace github.com/learningdog1/ada_link => ../ada_link
