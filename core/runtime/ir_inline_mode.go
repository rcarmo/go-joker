package runtime

import "os"

func IRInlineMode() string {
	mode := os.Getenv("JOKER_IR_INLINE")
	if mode == "" {
		return "auto"
	}
	return mode
}

func IRInlineForce() bool {
	mode := IRInlineMode()
	return mode == "1" || mode == "force" || mode == "all"
}

func IRInlineDisabled() bool {
	mode := IRInlineMode()
	return mode == "0" || mode == "off" || mode == "false"
}
