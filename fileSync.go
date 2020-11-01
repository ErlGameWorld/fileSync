package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	//"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SendDur  = 2
	SleepDur = 86400
)

var CollectFiles map[string]struct{}
var SendTimer *time.Timer
var Str bytes.Buffer
var Msg *bytes.Buffer
var AddDirs []string
var OnlyDirs []string
var DelDirs []string
var Conn net.Conn

const (
	hrl  = ".hrl"
	erl  = ".erl"
	beam = ".beam"
	dtl  = ".dtl"
	lfe  = "lfe"
	ex   = "ex"
	idea = ".idea"
	svn = ".svn"
	git = ".git"
	lock = ".lock"
	bea = ".bea"
)

type Watch struct {
	watch *fsnotify.Watcher
}

// 收集更改了的文件
func CollectFile(File string) {
	ext := filepath.Ext(File)
	if ext != idea && ext != git && ext != svn && ext != lock && ext != bea && ext != "" && ext != idea && (ext == erl || ext == beam || ext == hrl || ext == ex || ext == dtl || ext == lfe) {
		CollectFiles[File] = struct{}{}
		SendTimer.Reset(time.Second * SendDur)
	}
}

// 发送文件列表到erl层
func SendToErl() {
	// 拼写数据
	for k := range CollectFiles {
		Str.WriteString(k)
		Str.WriteString("\r\n")
	}
	CollectFiles = map[string]struct{}{}

	var length = int32(len(Str.Bytes()))
	//写入消息头
	_ = binary.Write(Msg, binary.BigEndian, length)
	//写入消息体
	_ = binary.Write(Msg, binary.BigEndian, Str.Bytes())
	Conn.Write(Msg.Bytes())
	Str.Reset()
	Msg.Reset()

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

func isOnlyDir(dirs []string, curDirs string) bool {
	cnt := 0
	for _, v := range dirs {
		if v != "" {
			cnt += 1
			if strings.Contains(curDirs[1:], v[1:]) {
				return true
			}
		}
	}
	if cnt == 0 {
		return true
	}
	return false
}

func isDelDir(dirs []string, curDirs string) bool {
	for _, v := range dirs {
		if v != "" {
			if strings.Contains(curDirs[1:], v[1:]) {
				return true
			}
		}
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
				if isOnlyDir(OnlyDirs, path) {
					if !isDelDir(DelDirs, path) {
						err = w.watch.Add(path)
						if err != nil {
							// fmt.Println("watch err : ", path, err)
							return err
						}
						// fmt.Println("watch success : ", path)
					}
				}
			}
		}
		return nil
	})

	for _, v := range AddDirs {
		if v != "" {
			//通过Walk来遍历目录下的所有子目录
			filepath.Walk(v, func(path string, info os.FileInfo, err error) error {
				//这里判断是否为目录，只需监控目录即可 目录下的文件也在监控范围内，不需要我们一个一个加
				if info.IsDir() {
					path, err := filepath.Abs(path)
					if err != nil {
						return err
					}
					if !isHidden(path) {
						err = w.watch.Add(path)
						if err != nil {
							// fmt.Println("watch AddDirs err : ", err)
							return err
						}
						// fmt.Println("watch AddDirs success : ", path)
					}
				}
				return nil
			})
		}
	}

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
							// fmt.Println("创建文件夹 : ", ev.Name)
							if !isHidden(ev.Name) {
								if isOnlyDir(OnlyDirs, ev.Name) {
									if !isDelDir(DelDirs, ev.Name) {
										w.watch.Add(ev.Name)
										// fmt.Println("添加监控 : ", ev.Name)
									}
								}
							}
						} else {
							// 新建了文件
							CollectFile(ev.Name)
							// fmt.Println("创建文件 : ", ev.Name)
						}
					}
					if ev.Op&fsnotify.Write == fsnotify.Write {
						CollectFile(ev.Name)
						// fmt.Println("写入文件 : ", ev.Name)
					}
					if ev.Op&fsnotify.Remove == fsnotify.Remove {
						// fmt.Println("删除文件 : ", ev.Name)
						//如果删除文件是目录，则移除监控
						fi, err := os.Stat(ev.Name)
						if err == nil && fi.IsDir() {
							w.watch.Remove(ev.Name)
							// fmt.Println("删除监控 : ", ev.Name)
						}
					}
					if ev.Op&fsnotify.Rename == fsnotify.Rename {
						// fmt.Println("重命名文件 : ", ev.Name)
						//如果重命名文件是目录，则移除监控
						//注意这里无法使用os.Stat来判断是否是目录了
						//因为重命名后，go已经无法找到原文件来获取信息了
						//所以这里就简单粗爆的直接remove好了
						w.watch.Remove(ev.Name)
					}
					if ev.Op&fsnotify.Chmod == fsnotify.Chmod {
						// fmt.Println("修改权限 : ", ev.Name)
					}
				}
			case <-w.watch.Errors:
				{
					// fmt.Println("error : ", err)
					return
				}
			case <-SendTimer.C:
				SendToErl()
			}
		}
	}()
}

func read(reader *bufio.Reader) ([]byte, error) {
	// Peek 返回缓存的一个切片，该切片引用缓存中前 n 个字节的数据，
	// 该操作不会将数据读出，只是引用，引用的数据在下一次读取操作之
	// 前是有效的。如果切片长度小于 n，则返回一个错误信息说明原因。
	// 如果 n 大于缓存的总大小，则返回 ErrBufferFull。
	lengthByte, err := reader.Peek(4)
	if err != nil {
		return nil, err
	}
	//创建 Buffer缓冲器
	lengthBuff := bytes.NewBuffer(lengthByte)
	var length int32
	// 通过Read接口可以将buf中得内容填充到data参数表示的数据结构中
	err = binary.Read(lengthBuff, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}
	// Buffered 返回缓存中未读取的数据的长度
	if int32(reader.Buffered()) < length+4 {
		return nil, err
	}
	// 读取消息真正的内容
	pack := make([]byte, int(4+length))
	// Read 从 b 中读出数据到 p 中，返回读出的字节数和遇到的错误。
	// 如果缓存不为空，则只能读出缓存中的数据，不会从底层 io.Reader
	// 中提取数据，如果缓存为空，则：
	// 1、len(p) >= 缓存大小，则跳过缓存，直接从底层 io.Reader 中读
	// 出到 p 中。
	// 2、len(p) < 缓存大小，则先将数据从底层 io.Reader 中读取到缓存
	// 中，再从缓存读取到 p 中。
	_, err = reader.Read(pack)
	if err != nil {
		return nil, err
	}
	return pack[4:], nil
}

func main() {
	CollectFiles = map[string]struct{}{}
	SendTimer = time.NewTimer(time.Second * SleepDur)
	defer SendTimer.Stop()

	Addr := "localhost:" + os.Args[2]
	var err error
	Conn, err = net.Dial("tcp", Addr)
	if err != nil {
		//fmt.Println("IMY****************建立tcp失败 : ", Addr)
		return
	}

	// 建立tcp 连接后需要从erlSync 接受监听目录相关配置
	var reader *bufio.Reader
	reader = bufio.NewReader(Conn)
	data, err := read(reader)
	if err == io.EOF {
		//fmt.Println("IMY****************Tcp 断开链接 : ", Addr)
		return
	}
	if err != nil {
		//fmt.Println("IMY****************Tcp read err : ", err)
		return
	}
	//fmt.Println("IMY****************建立tcp成功 : ", os.Args[0])
	//fmt.Println("IMY****************Tcp read data : ", string(data))
	dirs := strings.Split(string(data), "\r\n")
	AddDirs = strings.Split(dirs[0], "|")
	OnlyDirs = strings.Split(dirs[1], "|")
	DelDirs = strings.Split(dirs[2], "|")

	Msg = new(bytes.Buffer)
	watch, _ := fsnotify.NewWatcher()
	w := Watch{watch: watch}
	w.watchDir(os.Args[1])

	data = make([]byte, 10)
	_, _ = Conn.Read(data)
	Conn.Close()
	// fmt.Println("IMY****************tcp关闭了 : ", os.Args[0])
}
