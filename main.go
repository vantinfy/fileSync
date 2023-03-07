package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"log"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"gopkg.in/yaml.v3"
)

type FileSyncConfig struct {
	SubscribeName string   `yaml:"subscribe_name" comment:"订阅组 文件只会广播到订阅了相同组别的节点上"`
	SavePath      string   `yaml:"save_path" comment:"文件保存目录"`
	PriKey        string   `yaml:"pri_key" comment:"十六进制ecc密钥 曲线统一P256"`
	NodeId        string   `yaml:"node_id" comment:"节点id 与密钥有关"`
	Port          int      `yaml:"port" comment:"p2p端口"`
	Interval      int64    `yaml:"interval" comment:"同步间隔 单位毫秒"`
	Peers         []string `yaml:"peers" comment:"要连接的p2p邻居节点"`
	SyncFile      []string `yaml:"sync_file" comment:"需要同步的文件列表文件名"` // 如有运行期间更新sync_file的需求 使用GenYaml方法即可
}

const (
	configFileName = "config.yaml"
)

var (
	config FileSyncConfig
	ctx    context.Context
)

func readConfig() {
	configFileBytes, err := os.ReadFile(configFileName)
	if err != nil {
		log.Println("read config failed", err)
		return
	}
	err = yaml.Unmarshal(configFileBytes, &config)
	if err != nil {
		log.Println("yaml unmarshal failed", err)
	}
	validConfig(config)

	if _, exist := os.Stat(config.SavePath); exist != nil {
		err = os.Mkdir(config.SavePath, 0644)
		if err != nil {
			log.Println("mkdir failed", err)
		}
	}
}

// 检查配置参数是否合法
func validConfig(config FileSyncConfig) {}

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

func main() {
	ctx = context.Background()
	readConfig()

	// 加载ecc密钥(统一使用P256)用于后续对应p2p节点id
	priv, err := loadKey()
	if err != nil {
		log.Println("reduce ecc key failed", err)
		return
	}
	connManager, _ := connmgr.NewConnManager(100, 400)

	h, err := libp2p.New(libp2p.Identity(priv),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", config.Port)),
		libp2p.ConnectionManager(connManager),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
	)
	if err != nil {
		panic(err)
	}
	config.NodeId = h.ID().String()
	GenYaml(config, configFileName)
	log.Println("host id", h.ID(), h.Addrs())

	go connectPeers(ctx, h)

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}
	topic, err := ps.Join(config.SubscribeName)
	if err != nil {
		panic(err)
	}
	go PublishFile(ctx, topic)

	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	SaveFile(ctx, sub) // todo 这里加go协程就会swarm close
}

func loadKey() (crypto.PrivKey, error) {
	var keyBytes []byte
	if config.PriKey == "" {
		// 不存在则创建ecc密钥对 并存到文件中
		priK, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		keyBytes, _ = x509.MarshalECPrivateKey(priK)
		// 私钥序列化转16进制存yaml
		config.PriKey = hex.EncodeToString(keyBytes)
		GenYaml(config, configFileName)
	} else {
		var err error
		keyBytes, err = hex.DecodeString(config.PriKey)
		if err != nil {
			return nil, err
		}
	}
	return crypto.UnmarshalECDSAPrivateKey(keyBytes)
}

func connectPeers(ctx context.Context, h host.Host) {
	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		log.Println("Searching for peers...")
		for _, peerStr := range config.Peers {
			neighbor, err := peer.AddrInfoFromString(peerStr)
			if err != nil {
				panic("addr info from string failed" + err.Error())
			}
			if neighbor.ID == h.ID() {
				log.Println("self, continue")
				continue // No self connection
			}
			err = h.Connect(ctx, *neighbor)
			if err != nil {
				log.Println("Failed connecting to ", neighbor.ID, ", error:", err)
			} else {
				log.Println("Connected to:", neighbor.ID)
				anyConnected = true
			}
		}
		time.Sleep(time.Second * 3)
	}
	log.Println("Peer discovery complete")
}

func PublishFile(ctx context.Context, topic *pubsub.Topic) {
	ticker := time.NewTicker(time.Millisecond * time.Duration(config.Interval))
	defer ticker.Stop()
	for {
		<-ticker.C
		for _, syncFile := range config.SyncFile {
			fileContent, err := os.ReadFile(syncFile)
			if err != nil {
				log.Println("publish open file failed", err)
				// continue
				return
			}
			// 自行组装发送文件格式 [offset-表示文件名长度偏移量]-[syncFilenameBytes-文件名转bytes]-[fileContent-文件内容bytes]
			offset := []byte{uint8(len([]byte(syncFile)))}
			content := append(offset, []byte(syncFile)...)
			err = topic.Publish(ctx, append(content, fileContent...))
			if err != nil {
				log.Println("publish failed", err)
			} else {
				//log.Println("publish success", err)
			}
		}
	}
}

func SaveFile(ctx context.Context, sub *pubsub.Subscription) {
	for {
		m, err := sub.Next(ctx)
		if err != nil { // err其实就是监听ctx.done管道是否关闭
			log.Println("subscribe next error", err)
			return
		}
		if m.ReceivedFrom.String() == config.NodeId {
			// 判断从订阅中读取到新msg时，发送者是否为自己
			continue
		}
		if len(m.Message.Data) <= 1 {
			log.Println("length of m.Msg.Data <= 1")
			return
		}

		// 获取文件名偏移量
		offset := m.Message.Data[0]
		// 根据偏移量取得文件名
		filename := string(m.Message.Data[1 : offset+1])
		wfile, err := os.OpenFile(path.Join(config.SavePath, filename), os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			log.Println("save file open", err)
			return
		}
		// 写文件
		_, err = wfile.Write(m.Message.Data[offset+1:])
		if err != nil {
			log.Println("save file failed", err)
			_ = wfile.Close()
			return
		}
		log.Println("save file success", filename)
		_ = wfile.Close()
	}
}
