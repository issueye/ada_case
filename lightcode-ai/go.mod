module lightcode.ai/client

go 1.27.3

// The editor's assistant as its own project: it speaks the protocol on its
// standard streams and owns the network call. It imports the link library
// (the merged ada_link) for the message layer and nothing from the editor.
require github.com/learningdog1/ada_link v0.0.0

replace github.com/learningdog1/ada_link => ../ada_link
