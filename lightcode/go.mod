module lightcode

go 1.27.3

require (
	github.com/egoist/mygo v0.7.4
	github.com/learningdog1/ada_link v0.0.0
	golang.org/x/text v0.43.0
	lightcode.ai/client v0.0.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
)

// The framework is vendored from the local checkout while it moves towards
// 1.0; bump the commit here when upgrading (see docs/dev-plan.md §7).
replace github.com/egoist/mygo => ../../mygo

// The assistant plugin is its own project, and the IPC library is merged
// into this repository: both are replaced rather than fetched.
replace github.com/learningdog1/ada_link => ../ada_link

replace lightcode.ai/client => ../lightcode-ai
