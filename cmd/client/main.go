package main

import (
	"LEPG/internal/client"
	"LEPG/internal/config"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cfgFile string
var flagServerUrl string
var flagPort int

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
		// 收集命令行参数
		flagValues := make(map[string]any)
		if flagServerUrl != "" {
			flagValues["server"] = flagServerUrl
		}
		if flagPort != 0 {
			flagValues["port"] = flagPort
		}

		// 创建 providers（包含客户端默认值）
		providers := client.NewProviders(flagValues, cfgFile)

		// 初始化客户端配置
		cfg, err := client.InitClientConfig(providers.Chain)
		if err != nil {
			fmt.Printf("Failed to init client config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Client config: %+v\n", cfg)

		if err := client.MainFunc(cfg); err != nil {
			fmt.Printf("Client error: %v\n", err)
			os.Exit(1)
		}
	},
}

// Execute adds all child commands to the rootCmd and sets flags appropriately.
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
		defaults := client.GetDefaultValues()
		filename := "config.toml"
		if cfgFile != "" {
			filename = cfgFile
		}

		// 检查文件是否已存在
		if _, err := os.Stat(filename); err == nil {
			fmt.Printf("Config file already exists at %s. Please delete it before initializing.\n", filename)
			os.Exit(1)
		}

		if err := config.InitConfigWithDefaults(filename, defaults); err != nil {
			fmt.Printf("Failed to init config: %v\n", err)
			os.Exit(1)
		}

		absPath, _ := filepath.Abs(filename)
		fmt.Printf("LEPG client initialized successfully. Config file created at: %s\n", absPath)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default is ./config.toml)")
	runCmd.Flags().StringVarP(&flagServerUrl, "url", "u", "", "server URL (overrides config file)")
	runCmd.Flags().IntVarP(&flagPort, "port", "p", 0, "server port (overrides config file)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
}

func main() {
	Execute()
}
