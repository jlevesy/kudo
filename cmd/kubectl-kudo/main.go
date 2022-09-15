package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := cobra.Command{
		Use:           "kudo", // This is prefixed by kubectl in the custom usage template
		Short:         "kudo is the cloud native privilege escalation tool",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	rootCmd.AddCommand(newEscalateCmd())

	rootCmd.SetUsageTemplate(
		strings.NewReplacer(
			"{{.UseLine}}", "kubectl {{.UseLine}}",
			"{{.CommandPath}}", "kubectl {{.CommandPath}}",
		).Replace(rootCmd.UsageTemplate()),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
