package main

import (
	"LEPG/internal/client"
	"LEPG/internal/config"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lepgc",
	Short: "Client for LEPG",
	Long:  `LEPG client is a lightweight IoT gateway provides high performance, low power consumption, and easy to use.`,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run LEPG client",
	Long:  `Run LEPG client to start input and upload loops.`,
	Run: func(cmd *cobra.Command, args []string) {
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

		if err := config.CheckConfig(config.Client); err != nil {
			fmt.Printf("Config validation failed: %v\n", err)
			os.Exit(1)
		}

		var wg sync.WaitGroup
		wg.Go(func() {
			if err := client.InputLoop(); err != nil {
				fmt.Printf("Input loop error: %v\n", err)
			}
		})
		wg.Go(func() {
			if err := client.UploadLoop(); err != nil {
				fmt.Printf("Upload loop error: %v\n", err)
			}
		})
		wg.Wait()
	},
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

		config.InitConfig(config.Client)
		if err := config.CheckConfig(config.Client); err != nil {
			fmt.Printf("Config validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("LEPG client initialized successfully.")
	},
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default is ./config.toml or ./config/config.toml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
}

func main() {
	Execute()
}
