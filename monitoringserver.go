package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	configFile    string
	monitorDirs   []string
	hashDBFile    string
	logFilePath   string
	checkInterval time.Duration
	hashDB        = make(map[string]string)
	logFile       *os.File
	exclude       []string
	MaxFileSize   int64
	appversion    string
)

type Config struct {
	Wenjian struct {
		Directories []string `json:"directories"`
		Exclude     []string `json:"exclude"`
	} `json:"wenjian"`

	HashDBFile    string `json:"hash_db_file"`
	LogFile       string `json:"log_file"`
	CheckInterval string `json:"check_interval"`
}

func init() {
	flag.StringVar(&configFile, "config", "data/config.json", "Path to configuration file (JSON format)")
	flag.StringVar(&hashDBFile, "db", "data/hashdb.json", "Path to hash database file")
	flag.StringVar(&logFilePath, "log", "data/webmonitor.log", "Path to log file")

	flag.DurationVar(&checkInterval, "interval", 20*time.Minute, "Check interval (e.g. 5m, 1h)")
}

func main() {
	// 解析命令行参数
	flag.Parse()

	// 处理额外指定的目录参数
	args := flag.Args()
	if len(args) > 0 {
		monitorDirs = append(monitorDirs, args...)
	}

	appversion = "Webserver文件防篡改监控-秋裤子1.2版"
	initLog()
	defer logFile.Close()

	log.Println(appversion)

	// 加载配置
	if configFile != "" {
		loadConfigFromFile()
	} else {
		log.Println("未指定配置文件，使用命令行参数")
	}

	// 确保至少有一个监控目录
	if len(monitorDirs) == 0 {
		log.Fatal("错误：未指定任何监控目录")
	}

	log.Printf("监控目录: %v\n", monitorDirs)
	log.Printf("检查间隔: %v\n", checkInterval)
	log.Printf("哈希数据库文件: %s\n", hashDBFile)
	log.Printf("日志文件: %s\n", logFilePath)

	// 初始化哈希数据库
	initHashDB()

	// 确保程序退出时保存哈希数据库
	defer saveHashDB()

	// 开始监控
	startMonitoring()
}

func initLog() {
	// 创建日志目录
	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		log.Fatalf("无法创建日志目录: %v", err)
	}

	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("无法打开日志文件:", err)
	}
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
}

func loadConfigFromFile() {
	file, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("无法读取配置文件: %v", err)
	}

	var config Config
	if err := json.Unmarshal(file, &config); err != nil {
		log.Fatalf("解析配置文件错误: %v", err)
	}

	if len(config.Wenjian.Directories) == 0 {
		log.Fatalf("配置文件中必须指定至少一个监控目录: %v", err)
	}
	monitorDirs = config.Wenjian.Directories
	exclude = config.Wenjian.Exclude
	MaxFileSize = 10485760

	if config.HashDBFile != "" {
		hashDBFile = config.HashDBFile
	}

	if config.LogFile != "" {
		logFilePath = config.LogFile
	}

	if config.CheckInterval != "" {
		duration, err := time.ParseDuration(config.CheckInterval)
		if err != nil {
			log.Printf("无效的检查间隔 '%s', 使用默认值: %v", config.CheckInterval, err)
		} else {
			checkInterval = duration
		}
	}
}

func initHashDB() {
	// 尝试从文件加载已有的哈希数据库
	if _, err := os.Stat(hashDBFile); err == nil {
		file, err := os.ReadFile(hashDBFile)
		if err != nil {
			log.Printf("无法读取哈希数据库文件: %v", err)
		} else {
			if err := json.Unmarshal(file, &hashDB); err != nil {
				log.Printf("解析哈希数据库错误: %v", err)
			} else {
				log.Printf("从文件加载了 %d 个文件的哈希值", len(hashDB))
				return
			}
		}
	}

	// 如果无法加载，则重新初始化
	log.Println("初始化新的哈希数据库...")
	for _, dir := range monitorDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				hash, err := calculateFileHash(path)
				if err != nil {
					log.Printf("计算文件哈希错误 %s: %v\n", path, err)
					return nil
				}
				hashDB[path] = hash

			}
			return nil
		})

		if err != nil {
			log.Printf("遍历目录错误 %s: %v\n", dir, err)
		}
	}

	// 保存初始哈希数据库
	if err := saveHashDB(); err != nil {
		log.Printf("保存哈希数据库错误: %v", err)
	}

	log.Println("哈希数据库初始化完成")
}

func saveHashDB() error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(hashDBFile), 0755); err != nil {
		return fmt.Errorf("无法创建哈希数据库目录: %v", err)
	}

	data, err := json.MarshalIndent(hashDB, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化哈希数据库错误: %v", err)
	}

	if err := os.WriteFile(hashDBFile, data, 0644); err != nil {
		return fmt.Errorf("写入哈希数据库文件错误: %v", err)
	}

	return nil
}

func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func startMonitoring() {
	log.Printf("开始监控文件变化，检查间隔: %v...\n", checkInterval)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 立即执行一次检查
	checkFiles()

	for range ticker.C {
		checkFiles()
	}
}

func checkFiles() {
	log.Println(appversion + " 开始文件检查..")
	changesDetected := false

	for _, dir := range monitorDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// 跳过目录本身，只检查目录内容
			if path == dir {
				return nil
			}

			// 检查是否应该排除该文件/目录
			if shouldExclude(path, exclude) {
				if info.IsDir() {
					return filepath.SkipDir // 跳过整个目录
				}

				return nil // 跳过单个文件
			}

			// 只处理普通文件（跳过目录、符号链接等）
			if !info.Mode().IsRegular() {
				return nil
			}

			// 检查文件大小限制
			if MaxFileSize > 0 && info.Size() > MaxFileSize {

				return nil
			}

			currentHash, err := calculateFileHash(path)
			if err != nil {
				log.Printf("计算文件哈希错误 %s: %v\n", path, err)
				return nil
			}

			storedHash, exists := hashDB[path]

			if !exists {
				// 新文件
				hashDB[path] = currentHash
				alert(fmt.Sprintf("发现新文件: %s\n大小: %d bytes\n哈希: %s",
					path, info.Size(), currentHash))
				changesDetected = true
			} else if storedHash != currentHash {
				// 文件被修改
				hashDB[path] = currentHash
				alert(fmt.Sprintf("文件被修改: %s\n大小: %d bytes\n原哈希: %s\n新哈希: %s",
					path, info.Size(), storedHash, currentHash))
				changesDetected = true
			}

			return nil
		})

		if err != nil {
			log.Printf("遍历目录错误 %s: %v\n", dir, err)
		}
	}

	// 检查是否有文件被删除（同时考虑排除规则）
	for path := range hashDB {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// 检查被删除的文件是否在排除列表中
			if !shouldExclude(path, exclude) {
				delete(hashDB, path)
				alert(fmt.Sprintf("文件被删除: %s", path))
				changesDetected = true
			}
		}
	}

	if changesDetected {
		if err := saveHashDB(); err != nil {
			log.Printf("保存哈希数据库错误: %v", err)
		}
	}

	log.Println("文件检查完成 -.-")
}

func alert(message string) {
	// 记录到日志
	now := time.Now()
	riqi := now.Format("2006-01-02 15:04:05") + " "
	log.Println("警报:", riqi+message)

}
func shouldExclude(path string, excludePatterns []string) bool {
	// 统一使用斜杠路径分隔符，避免Windows反斜杠问题
	normalizedPath := filepath.ToSlash(path)

	for _, pattern := range excludePatterns {
		// 处理目录排除 (以/结尾的模式)
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if strings.HasPrefix(normalizedPath, dirPattern+"/") {
				return true
			}
			continue
		}

		// 处理通配符匹配
		if strings.Contains(pattern, "*") {
			// 匹配完整路径
			if match, _ := filepath.Match(pattern, filepath.Base(normalizedPath)); match {
				return true
			}
			continue
		}

		// 精确匹配完整路径
		if normalizedPath == pattern {
			return true
		}
	}
	return false
}
