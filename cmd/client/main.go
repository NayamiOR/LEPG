package main

import (
	"LEPG/internal/config"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lepgc",
	Short: "Client for LEPG",
	Long:  `LEPG client is a lightweight IoT gateway provides high performance, low power consumption, and easy to use.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize LEPG",
	Long:  `Initialize LEPG`,
	Run: func(cmd *cobra.Command, args []string) {
		config.InitConfig(config.Client)
		config.CheckConfig(config.Client)
	},
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.LEPG.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.AddCommand(initCmd)
}

func main() {
	Execute()
}
