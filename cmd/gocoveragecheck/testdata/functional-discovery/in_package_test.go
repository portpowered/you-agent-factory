package discoveryfixture

import (
	"testing"
	testingAlias "testing"
)

func TestValid(t *testing.T)                {}
func TestAlias(t *testingAlias.T)           {}
func TestNoParameter()                      {}
func TestWrongType(t testing.TB)            {}
func TestTwoParameters(t, other *testing.T) {}
func TestReturn(t *testing.T) bool          { return true }
func TestGeneric[T any](t *testing.T)       {}
func Test()                                 {}
func TestExample(t *testing.T)              {}

type fixture struct{}

func (fixture) TestMethod(t *testing.T) {}
