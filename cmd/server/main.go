package main

import (
	"LEPG/internal/config"
	"LEPG/internal/server"
	serverstore "LEPG/internal/server/cache"
	"context"
	"fmt"
	"os"
	"path/filepath"

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
		// 收集命令行参数
		flagValues := make(map[string]any)
		if flagPort != 0 {
			flagValues["port"] = flagPort
		}

		// 创建 providers（包含服务端默认值）
		providers := server.NewProviders(flagValues, cfgFile)

		// 初始化服务端配置
		cfg, err := server.InitServerConfig(providers.Chain)
		if err != nil {
			fmt.Printf("Failed to init server config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Server config: %+v\n", cfg)

		// 打印客户端列表
		fmt.Printf("Loaded %d clients:\n", len(cfg.Clients))
		for i, client := range cfg.Clients {
			fmt.Printf("  [%d] SN: %s, Token: %s, Description: %s\n",
				i+1, client.Sn, client.Token, client.Description)
		}

		store, err := serverstore.NewSQLiteStore(context.Background(), cfg.DataPath)
		if err != nil {
			fmt.Printf("Failed to create store: %v\n", err)
			os.Exit(1)
		}
		defer store.Close()

		// 创建并启动 MQTT broker
		broker := server.NewMqttBroker(&cfg.Mqtt)
		if err := broker.Start(); err != nil {
			fmt.Printf("Failed to start MQTT broker: %v\n", err)
			os.Exit(1)
		}
		defer broker.Stop()

		// TODO: 数据桥接阶段替换为 server.NewMqttPublisher(broker)
		var publisher server.EventPublisher = new(server.NopPublisher)

		if err := server.ReceiveLoop(cfg, store, publisher); err != nil {
			fmt.Printf("Server error: %v\n", err)
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
		defaults := server.GetDefaultValues()
		filename := "config/server.toml"
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
		fmt.Printf("LEPG server initialized successfully. Config file created at: %s\n", absPath)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default is ./config/server.toml)")
	runCmd.Flags().IntVarP(&flagPort, "port", "p", 0, "server port (overrides config file)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
}

func main() {
	Execute()
}
