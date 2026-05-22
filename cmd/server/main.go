package main

import (
	"LEPG/internal/config"
	"LEPG/internal/server"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
)

var cfgFile string
var flagPort int

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lepgs",
	Short: "Server for LEPG",
	Long:  `LEPG server is a lightweight IoT gateway provides high performance, low power consumption, and easy to use.`,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run LEPG server",
	Long:  `Run LEPG server to start receive and process loops.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Set flag values first (higher priority than config file)
		config.SetFlagValues("", flagPort)

		if cfgFile == "" {
			if err := config.LoadConfig(); err != nil {
				fmt.Printf("Failed to load config: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := config.LoadConfigWithPath(cfgFile); err != nil {
				fmt.Printf("Failed to load config from %s: %v\n", cfgFile, err)
				os.Exit(1)
			}
		}

		// Unmarshal server config
		cfg, err := server.UnmarshalServerConfigFromViper()
		if err != nil {
			fmt.Printf("Failed to unmarshal server config: %v\n", err)
			os.Exit(1)
		}
		server.SetServerConfig(cfg)

		if err := server.CheckConfigNotSet(); err != nil {
			fmt.Printf("Config validation failed: %v\n", err)
			os.Exit(1)
		}

		var wg sync.WaitGroup
		wg.Go(func() {
			if err := server.ReceiveLoop(); err != nil {
				fmt.Printf("Receive loop error: %v\n", err)
			}
		})

		wg.Wait()
	},
}

// Execute adds all child commands to the rootCmd and sets flags appropriately.
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
		if cfgFile == "" {
			if err := config.LoadConfig(); err == nil {
				fmt.Println("Config file already exists. Please delete config.toml before initializing.")
				os.Exit(1)
			}
		} else {
			if err := config.LoadConfigWithPath(cfgFile); err == nil {
				fmt.Printf("Config file already exists at %s. Please delete it before initializing.\n", cfgFile)
				os.Exit(1)
			}
		}

		config.InitConfig(server.GetDefaultValues())
		if err := server.CheckConfigNotSet(); err != nil {
			fmt.Printf("Config validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("LEPG server initialized successfully.")
	},
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default is ./config.toml or ./config/config.toml)")

	// Server-specific flags for run command
	runCmd.Flags().IntVarP(&flagPort, "port", "p", 0, "server port (overrides config file)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
}

func main() {
	Execute()
}
