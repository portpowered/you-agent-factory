package baseline

import "testing"

func TestCommandPathPresent(t *testing.T) {
	tree := "you factory create\tcreate <name>\tyou factory\nyou factory\tfactory\tyou\n"

	if !commandPathPresent(tree, "you factory create") {
		t.Fatal("expected factory create path present")
	}
	if commandPathPresent(tree, "you factory save") {
		t.Fatal("expected removed path absent")
	}
}

func TestRunFlagPresent(t *testing.T) {
	flags := "port\tdeprecated\nserver\tAPI base\n"

	if !runFlagPresent(flags, "server") {
		t.Fatal("expected server flag present")
	}
	if runFlagPresent(flags, "nosuch") {
		t.Fatal("expected missing flag absent")
	}
}
