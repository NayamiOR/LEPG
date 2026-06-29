package main

import (
	"LEPG/internal/config"
	logging "LEPG/internal/log"
	"LEPG/internal/output"
	"LEPG/internal/server"
	serverstore "LEPG/internal/server/cache"
	"LEPG/internal/server/cache/connections"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/SladkyCitron/slogcolor"
	"github.com/redis/go-redis/v9"
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

		// 配置持久化日志（控制台 + 文件双输出）
		if err := logging.Setup(logging.Config{
			Level:      cfg.LogLevel,
			Path:       cfg.LogPath,
			MaxSize:    cfg.LogMaxSize,
			MaxBackups: cfg.LogMaxBackups,
			MaxAge:     cfg.LogMaxAge,
			AppName:    "lepgs",
		}); err != nil {
			fmt.Printf("Failed to setup logging: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.Clients) == 0 {
			slog.Warn("No clients configured. Server will not receive any data.")
		} else {
			// 打印客户端列表
			slog.Info("Loaded clients", "count", len(cfg.Clients))
			for i, client := range cfg.Clients {
				fmt.Printf("  [%d] SN: %s, Token: %s, Description: %s\n",
					i+1, client.Sn, client.Token, client.Description)
			}
		}

		dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			cfg.Pg.User, cfg.Pg.Password, cfg.Pg.Host, cfg.Pg.Port, cfg.Pg.DBName, cfg.Pg.SSLMode)
		store, err := serverstore.NewPostgresStore(context.Background(), dsn)
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

		// Redis 连接状态管理
		rdb := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			fmt.Printf("Failed to connect to Redis at %s: %v\n", cfg.Redis.Addr, err)
			os.Exit(1)
		}
		defer rdb.Close()
		connMgr := connections.NewRedisConnectionManager(rdb)

		// 创建 OutputRouter（对外 Push 模式）
		var router *output.OutputRouter
		if len(cfg.Outputs) > 0 {
			sinks := make([]output.Sinker, 0, len(cfg.Outputs))
			for _, outCfg := range cfg.Outputs {
				if !outCfg.Enabled {
					continue
				}
				sink, err := output.NewSinker(outCfg)
				if err != nil {
					slog.Warn("failed to create sinker, skipping", "name", outCfg.Name, "error", err)
					continue
				}
				sinks = append(sinks, sink)
				slog.Info("output sinker created", "name", outCfg.Name, "type", outCfg.Type)
			}
			if len(sinks) > 0 {
				router = output.NewOutputRouter(sinks)
				defer router.Shutdown()
			}
		}

		if err := server.ReceiveLoop(cfg, store, publisher, connMgr, router); err != nil {
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
		filename := defaults["config_path"].(string)
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
	slog.SetDefault(slog.New(slogcolor.NewHandler(os.Stderr, slogcolor.DefaultOptions)))
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default is ./config/server.toml)")
	runCmd.Flags().IntVarP(&flagPort, "port", "p", 0, "server port (overrides config file)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
}

func main() {
	Execute()
}
