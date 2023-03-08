package p2p

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"fileSync/config"
	"fmt"
	"github.com/libp2p/go-libp2p/core/crypto"
	"log"
)

func loadKey(globalConfig config.FileSyncConfig) (crypto.PrivKey, error) {
	var keyBytes []byte
	if globalConfig.PriKey == "" {
		// 不存在则创建ecc密钥对 并存到文件中
		priK, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		keyBytes, _ = x509.MarshalECPrivateKey(priK)
		// 私钥序列化转16进制存yaml
		globalConfig.PriKey = hex.EncodeToString(keyBytes)
		config.GenYaml(globalConfig, config.CfgFileName)
	} else {
		var err error
		keyBytes, err = hex.DecodeString(globalConfig.PriKey)
		if err != nil {
			return nil, err
		}
	}
	return crypto.UnmarshalECDSAPrivateKey(keyBytes)
}

func StartNetwork(ctx context.Context, globalConfig config.FileSyncConfig) {
	// 加载ecc密钥(统一使用P256)用于后续对应p2p节点id
	priv, err := loadKey(globalConfig)
	if err != nil {
		log.Println("reduce ecc key failed", err)
		return
	}

	n, err := NewNode(priv, fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", globalConfig.Port), ctx)
	if err != nil {
		log.Println("new p2p node failed", err)
		return
	}
	defer n.Close()
	// 订阅
	err = n.Join(globalConfig.SubscribeName)
	if err != nil {
		log.Printf("node join [%v] failed: %v\n", globalConfig.SubscribeName, err)
		return
	}

	// 将本节点id信息也写入文件
	globalConfig.NodeId = n.Host.ID().String()
	config.GenYaml(globalConfig, config.CfgFileName)
	log.Println("node started: ", n.Host.ID(), n.Host.Addrs())

	// 连接邻居
	go n.ConnectPeers(globalConfig.Peers)

	// 定期发送文件
	go n.PublishFile(globalConfig)

	n.SaveFile(globalConfig) // todo 这里改为开go协程就会swarm closed 导致无法连接其它节点
}
