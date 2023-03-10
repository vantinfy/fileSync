package p2p

import (
	"context"
	"fileSync/config"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"log"
	"os"
	"path"
	"time"
)

type Node struct {
	Host  host.Host // 核心
	Ctx   context.Context
	topic *pubsub.Topic
	sub   *pubsub.Subscription
}

func NewNode(priv crypto.PrivKey, listenAddr string, ctx context.Context) (*Node, error) {
	// prevent the peer from having too many connections by attaching a connection manager.
	connManager, _ := connmgr.NewConnManager(100, 400)
	// 创建p2p节点
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.ConnectionManager(connManager),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
	)

	return &Node{
		Host: h,
		Ctx:  ctx,
	}, err
}

// Join 可以拓展为join多个订阅 但是目前意义不大
func (n *Node) Join(subscribeName string) error {
	ps, err := pubsub.NewGossipSub(n.Ctx, n.Host)
	if err != nil {
		return err
	}
	topic, err := ps.Join(subscribeName)
	if err != nil {
		return err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return err
	}

	n.topic = topic
	n.sub = sub
	return nil
}

// ConnectPeers todo 后续可以做成定时检测邻居变动并连接的
func (n *Node) ConnectPeers(peers []string) {
	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		log.Println("Searching for peers...")
		for _, peerStr := range peers {
			neighbor, err := peer.AddrInfoFromString(peerStr)
			if err != nil {
				log.Printf("addr[%s] info from string failed: %v\n", peerStr, err)
				continue
			}
			if neighbor.ID == n.Host.ID() {
				continue // No self connection
			}
			// 尝试连接邻居
			err = n.Host.Connect(n.Ctx, *neighbor)
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

func (n *Node) PublishFile(globalConfig config.FileSyncConfig) {
	ticker := time.NewTicker(time.Millisecond * time.Duration(globalConfig.Interval))
	defer ticker.Stop()
	for {
		<-ticker.C
		for _, syncFile := range globalConfig.SyncFile {
			fileContent, err := os.ReadFile(syncFile)
			if err != nil {
				log.Println("publish open file failed", err)
				// continue
				return
			}
			// todo 定义一个结构 包含name content 存取时使用(un)marshal即可

			// 自行组装发送文件格式 [offset-表示文件名长度偏移量]-[syncFilenameBytes-文件名转bytes]-[fileContent-文件内容bytes]
			offset := []byte{uint8(len([]byte(syncFile)))}
			content := append(offset, []byte(syncFile)...)
			err = n.topic.Publish(n.Ctx, append(content, fileContent...))
			if err != nil {
				log.Println("publish failed", err)
			} else {
				//log.Println("publish success", err)
			}
		}
	}
}

func (n *Node) SaveFile(globalConfig config.FileSyncConfig) {
	for {
		m, err := n.sub.Next(n.Ctx)
		if err != nil { // err其实就是监听ctx.done管道是否关闭
			log.Println("node next error", err)
			return
		}
		if m.ReceivedFrom.String() == globalConfig.NodeId {
			// 判断从订阅中读取到新msg时，发送者是否为自己
			continue
		}
		if len(m.Message.Data) <= 1 {
			log.Println("length of m.Msg.Data <= 1")
			return
		}

		// 获取文件名偏移量
		offset := m.Message.Data[0]
		// 避免其它节点不按照PublishFile中的格式（[filename_offset-filename-file_content]）发布内容
		if int(offset+1) >= len(m.Message.Data) {
			log.Println("receive something not file:", string(m.Message.Data))
			continue
		}
		// 根据偏移量取得文件名
		filename := string(m.Message.Data[1 : offset+1])
		wfile, err := os.OpenFile(path.Join(globalConfig.SavePath, filename), os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			log.Println("save file open failed", err)
			continue
		}
		// 写文件
		_, err = wfile.Write(m.Message.Data[offset+1:])
		if err != nil {
			log.Println("save file failed", err)
			_ = wfile.Close()
			continue
		}
		log.Println("save file success", filename)
		_ = wfile.Close()
	}
}

func (n *Node) Close() {
	err := n.Host.Close()
	if err != nil {
		log.Println("node closed error", err)
	}
}
