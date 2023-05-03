package main

import (
	"flag"
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var updateFlag = flag.Bool("update", false, "update the second argument of any failing cmp commands in a testscript")

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(goModerateTestingMain{m}, map[string]func() int{
		"gomoderate": goModerateMain,
	}))
}

func TestScripts(t *testing.T) {
	p := testscript.Params{
		Dir:           "testscripts",
		UpdateScripts: *updateFlag,
		Setup: func(e *testscript.Env) error {
			e.Vars = append(e.Vars, "GOMODERATE_TEST_APPKEY="+os.Getenv("GOMODERATE_TEST_APPKEY"))
			return nil
		},
		// future:
		// Testscripts default to HOME=/no-home
		// Setup: func(e *testscript.Env) error {
		// 	home, err := os.UserHomeDir()
		// 	if err != nil {
		// 		log.Fatal(err)
		// 	}
		// 	return nil
		// },
	}
	testscript.Run(t, p)
}

type goModerateTestingMain struct {
	m *testing.M
}

func (m goModerateTestingMain) Run() int {
	// could do additional setup here if needed (e.g., check or set env vars, etc.)
	return m.m.Run()
}

// func homeEnvVar() string {
// 	if runtime.GOOS == "windows" {
// 		return "USERPROFILE"
// 	}
// 	return "HOME"
// }
