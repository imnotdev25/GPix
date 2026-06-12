package disguise

import _ "embed"

//go:embed wrapper.mp4
var wrapperMP4 []byte

func WrapperBytes() []byte { return wrapperMP4 }
