package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	SendDur  = 2
	SleepDur = 86400
)

var CollectFiles map[string]struct{}
var SendTimer *time.Timer
var Conn net.Conn

const (
	hrl = ".hrl"
	erl = ".erl"
	beam = ".beam"
	dtl = ".dtl"
	lfe = "lfe"
	ex  = "ex"
)

type Watch struct {
	watch *fsnotify.Watcher
}

// 收集更改了的文件
func CollectFile(File string) {
	ext := filepath.Ext(File)
	fmt.Println("IMY****************收集数据 : ", File)
	fmt.Println("IMY****************收集数据ext : ", ext)
	if ext != "" && (ext == erl || ext == beam || ext == hrl || ext == ex || ext == dtl || ext == lfe) {
		CollectFiles[File] = struct{}{}
		SendTimer.Reset(time.Second * SendDur)
		fmt.Println("IMY****************收集数据成功: ", File)
	} else {
		fmt.Println("IMY****************收集数据失败: ", File)
	}
}

// 发送文件列表到erl层
func SendToErl() {
	fmt.Println("IMY****************发送数据到tcp : ", CollectFiles)

	// 拼写数据
	var buffer bytes.Buffer
	for k := range CollectFiles {
		buffer.WriteString(k)
		buffer.WriteString("\r\n")
	}
	CollectFiles = map[string]struct{}{}

	var length = int32(len(buffer.Bytes()))
	var msg = new(bytes.Buffer)
	//写入消息头
	_ = binary.Write(msg, binary.BigEndian, length)
	//写入消息体
	_ = binary.Write(msg, binary.BigEndian, buffer.Bytes())
	fmt.Println("IMY****************发送数据到sock : ", msg)
	Conn.Write(msg.Bytes())

	SendTimer.Reset(time.Second * SleepDur)
}

func isHidden(path string) bool {
	for i := len(path) - 1; i > 0; i-- {
		if path[i] != '.' {
			continue
		}

		if os.IsPathSeparator(path[i-1]) {
			return true
		}
	}

	if path[0] == '.' {
		return true
	}

	return false
}

//监控目录
func (w *Watch) watchDir(dir string) {
	//通过Walk来遍历目录下的所有子目录
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		//这里判断是否为目录，只需监控目录即可 目录下的文件也在监控范围内，不需要我们一个一个加
		if info.IsDir() {
			path, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if !isHidden(path) {
				err = w.watch.Add(path)
				if err != nil {
					return err
				}
				fmt.Println("监控 : ", path)
			}
		}
		return nil
	})

	// 启动文件监听goroutine
	go func() {
		for {
			select {
			case ev := <-w.watch.Events:
				{
					if ev.Op&fsnotify.Create == fsnotify.Create {
						//这里获取新创建文件的信息，如果是目录，则加入监控中
						fi, err := os.Stat(ev.Name)
						if err == nil && fi.IsDir() {
							// 新建了文件夹
							fmt.Println("创建文件夹 : ", ev.Name)
							if !isHidden(ev.Name) {
								w.watch.Add(ev.Name)
								fmt.Println("添加监控 : ", ev.Name)
							}
						} else {
							// 新建了文件
							CollectFile(ev.Name)
							fmt.Println("创建文件 : ", ev.Name)
						}
					}
					if ev.Op&fsnotify.Write == fsnotify.Write {
						CollectFile(ev.Name)
						fmt.Println("写入文件 : ", ev.Name)
					}
					if ev.Op&fsnotify.Remove == fsnotify.Remove {
						fmt.Println("删除文件 : ", ev.Name)
						//如果删除文件是目录，则移除监控
						fi, err := os.Stat(ev.Name)
						if err == nil && fi.IsDir() {
							w.watch.Remove(ev.Name)
							fmt.Println("删除监控 : ", ev.Name)
						}
					}
					if ev.Op&fsnotify.Rename == fsnotify.Rename {
						fmt.Println("重命名文件 : ", ev.Name)
						//如果重命名文件是目录，则移除监控
						//注意这里无法使用os.Stat来判断是否是目录了
						//因为重命名后，go已经无法找到原文件来获取信息了
						//所以这里就简单粗爆的直接remove好了
						w.watch.Remove(ev.Name)
					}
					if ev.Op&fsnotify.Chmod == fsnotify.Chmod {
						fmt.Println("修改权限 : ", ev.Name)
					}
				}
			case err := <-w.watch.Errors:
				{
					fmt.Println("error : ", err)
					return
				}
			case <-SendTimer.C:
				SendToErl()
			}
		}
	}()
}

func main() {
	CollectFiles = map[string]struct{}{}
	SendTimer = time.NewTimer(time.Second * SleepDur)
	defer SendTimer.Stop()

	Addr := "localhost:" + os.Args[2]
	var err error
	Conn, err = net.Dial("tcp", Addr)
	if err != nil {
		fmt.Println("IMY****************建立tcp失败 : ", Addr)
		return
	}
	fmt.Println("IMY****************建立tcp成功 : ", os.Args[0])
	watch, _ := fsnotify.NewWatcher()
	w := Watch{watch: watch}
	w.watchDir(os.Args[1])

	data := make([]byte, 10)

	_, _ = Conn.Read(data)
	Conn.Close()
	fmt.Println("IMY****************建立tcp关闭了 : ", os.Args[0])
}
