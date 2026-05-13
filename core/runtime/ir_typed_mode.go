package runtime

import "os"

func IRTypedEnabled() bool {
	mode := os.Getenv("JOKER_IR_TYPED")
	return mode != "0" && mode != "off" && mode != "false"
}

func IRTypedMapMode() string {
	mode := os.Getenv("JOKER_IR_TYPED_MAP")
	if mode == "" {
		return "auto"
	}
	return mode
}

func IRTypedMapEnabled() bool {
	mode := IRTypedMapMode()
	return mode != "0" && mode != "off" && mode != "false"
}

func IRTypedVecEnabled() bool {
	mode := os.Getenv("JOKER_IR_TYPED_VEC")
	return mode == "1" || mode == "on" || mode == "true" || mode == "force"
}

func IRTypedMapForce() bool {
	mode := IRTypedMapMode()
	return mode == "1" || mode == "force" || mode == "all"
}
