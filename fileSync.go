package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	SendDur  = 1111     // 发送时间间隔毫秒
	SleepDur = 86400000 // 定期器初始睡眠时间
)

var CollectFiles map[string]struct{}
var SendTimer *time.Timer
var Str bytes.Buffer
var AddDirs []string
var OnlyDirs []string
var DelDirs []string
var LenBuff []byte

const (
	hrl    = ".hrl"
	erl    = ".erl"
	beam   = ".beam"
	dtl    = ".dtl"
	lfe    = "lfe"
	ex     = "ex"
	idea   = ".idea"
	svn    = ".svn"
	git    = ".git"
	lock   = ".lock"
	bea    = ".bea"
	config = ".config"
)

type Watch struct {
	watch *fsnotify.Watcher
}

const (
	//LOGPATH  LOGPATH/time.Now().Format(FORMAT)/*.log
	LOGPATH = "./"
	//FORMAT .
	FORMAT = "20060102"
	//LineFeed 换行
	LineFeed = "\r\n"
	FileName = "fileSync.log"
)

//以天为基准,存日志
var path = LOGPATH + time.Now().Format(FORMAT) + "_"

//WriteLog return error
func WriteLog(msg string) error {
	var (
		err error
		f   *os.File
	)

	f, err = os.OpenFile(path+FileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	_, err = io.WriteString(f, "Tag"+"::"+msg+LineFeed)

	defer f.Close()
	return err
}

//IsExist  判断文件夹/文件是否存在  存在返回 true
func IsExist(f string) bool {
	_, err := os.Stat(f)
	return err == nil || os.IsExist(err)
}

// 收集更改了的文件
func CollectFile(File string) {
	ext := filepath.Ext(File)
	if ext != idea && ext != git && ext != svn && ext != lock && ext != bea && ext != "" && ext != idea && (ext == erl || ext == beam || ext == hrl || ext == config || ext == ex || ext == dtl || ext == lfe) {
		CollectFiles[File] = struct{}{}
		SendTimer.Reset(time.Millisecond * SendDur)
	}
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

// 判断所给路径文件/文件夹是否存在
func existPath(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return true
	}
	return true
}

//监控目录
func (w *Watch) watchDir(dir string) {
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
							if !isHidden(ev.Name) {
								if isOnlyDir(OnlyDirs, ev.Name) {
									if !isDelDir(DelDirs, ev.Name) {
										w.watch.Add(ev.Name)
									}
								}
							}
						} else {
							// 新建了文件
							CollectFile(ev.Name)
						}
					}
					if ev.Op&fsnotify.Write == fsnotify.Write {
						CollectFile(ev.Name)
					}
					if ev.Op&fsnotify.Remove == fsnotify.Remove {
						//如果删除文件是目录，则移除监控
						fi, err := os.Stat(ev.Name)
						if err == nil && fi.IsDir() {
							w.watch.Remove(ev.Name)
						}
					}
					if ev.Op&fsnotify.Rename == fsnotify.Rename {
						//如果重命名文件是目录，则移除监控
						//注意这里无法使用os.Stat来判断是否是目录了
						//因为重命名后，go已经无法找到原文件来获取信息了
						//所以这里就简单粗爆的直接remove好了
						w.watch.Remove(ev.Name)
					}
					if ev.Op&fsnotify.Chmod == fsnotify.Chmod {
					}
				}
			case <-w.watch.Errors:
				{
					continue
				}
			case <-SendTimer.C:
				SendToErl()
			}
		}
	}()

	//通过Walk来遍历目录下的所有子目录
	filepath.WalkDir(dir, func(path string, info fs.DirEntry, err error) error {
		//这里判断是否为目录，只需监控目录即可 目录下的文件也在监控范围内，不需要我们一个一个加

		if err == nil && info.IsDir() {
			path, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if !isHidden(path) && isOnlyDir(OnlyDirs, path) && !isDelDir(DelDirs, path) {
				err = w.watch.Add(path)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})

	for _, v := range AddDirs {
		if v != "" && existPath(v) {
			//通过Walk来遍历目录下的所有子目录
			filepath.WalkDir(v, func(path string, info fs.DirEntry, err error) error {
				//这里判断是否为目录，只需监控目录即可 目录下的文件也在监控范围内，不需要我们一个一个加
				if err == nil && info.IsDir() {
					path, err := filepath.Abs(path)
					if err != nil {
						return err
					}
					if !isHidden(path) {
						err = w.watch.Add(path)
						if err != nil {
							return err
						}
					}
				}
				return nil
			})
		}
	}

	//for _, v := range AddDirs {
	//	if v != "" && existPath(v) {
	//		w.watch.Add(v)
	//	}
	//}
}

//********************************************** port start ************************************************************
func ReadLen() (int32, error) {
	if _, err := io.ReadFull(os.Stdin, LenBuff); err != nil {
		return 0, err
	}
	size := int32(binary.BigEndian.Uint32(LenBuff))

	return size, nil
}

func Read() ([]byte, error) {
	len, err := ReadLen()
	if err != nil {
		return nil, err
	} else if len == 0 {
		return []byte{}, nil
	}
	data := make([]byte, len)
	size, err := io.ReadFull(os.Stdin, data)
	return data[:size], err
}

func Write(data []byte) (int, error) {
	size := len(data)
	binary.BigEndian.PutUint32(LenBuff, uint32(size))

	if _, err := os.Stdout.Write(LenBuff); err != nil {
		return 0, err
	}

	return os.Stdout.Write(data)
}

// 发送文件列表到erl层
func SendToErl() {
	for k := range CollectFiles {
		Str.WriteString(k)
		Str.WriteString("\r\n")
	}
	CollectFiles = map[string]struct{}{}
	Write(Str.Bytes())
	Str.Reset()
}

//********************************************** port end   ************************************************************

func main() {
	CollectFiles = map[string]struct{}{}
	SendTimer = time.NewTimer(time.Millisecond * SleepDur)
	defer SendTimer.Stop()
	LenBuff = make([]byte, 4)

	Write([]byte("init"))
	data, err := Read()
	if err == io.EOF || err != nil {
		return
	}
	dirs := strings.Split(string(data), "\r\n")
	AddDirs = strings.Split(dirs[0], "|")
	OnlyDirs = strings.Split(dirs[1], "|")
	DelDirs = strings.Split(dirs[2], "|")
	watch, _ := fsnotify.NewWatcher()
	defer watch.Close()
	w := Watch{watch: watch}
	w.watchDir("./")
	Read()
}
