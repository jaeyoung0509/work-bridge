package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func placeholderRunE(name string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return newExitError(
			ExitNotImplemented,
			fmt.Sprintf("%s command is scaffolded but not implemented yet", name),
		)
	}
}
