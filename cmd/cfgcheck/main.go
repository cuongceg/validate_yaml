package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/pflag"

	"example.com/mqbridge-cfg/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	var cfgPath string
	var printNormalized bool

	pflag.StringVarP(&cfgPath, "config", "c", "", "path to config.yaml")
	pflag.BoolVar(&printNormalized, "print", false, "print normalized config")
	pflag.Parse()

	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "-c/--config is required")
		os.Exit(2)
	}

	b, err := ioutil.ReadFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	fmt.Println("CONFIG OK ✅")
	if printNormalized {
		out, _ := yaml.Marshal(cfg)
		fmt.Println("---")
		fmt.Print(string(out))
	}
}