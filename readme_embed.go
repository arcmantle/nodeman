package nodeman

import (
	_ "embed"
)

// EmbeddedREADME is a build-time snapshot of the repository README.
// It is used by `nodeman docs` to render local, versioned documentation.
//
//go:embed README.md
var EmbeddedREADME []byte

//go:embed assets/logo.svg
var EmbeddedLogoSVG []byte
