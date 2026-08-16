//go:build windows

package process

func spawnCommandHelperEscapedChildMode() {
	spawnCommandHelperChildMode("child-sleep")
}
