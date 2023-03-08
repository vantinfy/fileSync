package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"reflect"
)

const (
	CfgFileName = "config.yaml"
)

type FileSyncConfig struct {
	SubscribeName string   `yaml:"subscribe_name" comment:"订阅组 文件只会广播到订阅了相同组别的节点上"`
	SavePath      string   `yaml:"save_path" comment:"文件保存目录"`
	PriKey        string   `yaml:"pri_key" comment:"十六进制ecc密钥 曲线统一P256"`
	NodeId        string   `yaml:"node_id" comment:"节点id 与密钥有关"`
	Port          int      `yaml:"port" comment:"p2p端口"`
	Interval      int64    `yaml:"interval" comment:"同步间隔 单位毫秒"`
	Peers         []string `yaml:"peers" comment:"要连接的p2p邻居节点"`
	SyncFile      []string `yaml:"sync_file" comment:"需要同步的文件列表文件名"`
}

func ReadConfig() (FileSyncConfig, error) {
	config := FileSyncConfig{}

	configFileBytes, err := os.ReadFile(CfgFileName)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(configFileBytes, &config)
	if err != nil {
		return config, err
	}

	return config, validConfig(config)
}

// 检查配置参数是否合法
func validConfig(config FileSyncConfig) error {
	if _, exist := os.Stat(config.SavePath); exist != nil {
		err := os.Mkdir(config.SavePath, 0644)
		if err != nil {
			return fmt.Errorf("read config done, but mkdir failed: %v", err)
		}
	}
	return nil
}

func GenYaml(syncConfig FileSyncConfig, genPath string) {
	t := reflect.TypeOf(syncConfig)
	v := reflect.ValueOf(syncConfig)
	content := ""
	for i := 0; i < t.NumField(); i++ {
		comment := "# " + t.Field(i).Tag.Get("comment") + "\n"
		tag := t.Field(i).Tag.Get("yaml")
		switch v.Field(i).Kind() {
		case reflect.Slice:
			c := tag + ":\n"
			v.Field(i).Len()
			for j := 0; j < v.Field(i).Len(); j++ {
				c += " - " + v.Field(i).Index(j).String() + "\n"
			}
			comment += c
		case reflect.String:
			comment += tag + ": " + v.Field(i).String() + "\n"
		case reflect.Int, reflect.Int64:
			comment += fmt.Sprintf("%s: %d\n", tag, v.Field(i).Int())
		case reflect.Bool:
			comment += fmt.Sprintf("%s: %v\n", tag, v.Field(i).Bool())
		}
		content += comment
	}
	yamlFile, err := os.OpenFile(genPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Println("open file failed", err)
		return
	}
	_, err = yamlFile.Write([]byte(content))
	if err != nil {
		log.Println("write yaml file failed", err)
	}
	_ = yamlFile.Close()
}
