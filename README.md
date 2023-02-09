# fileSync 同步文件更改相关

# go windows下程序运行隐藏dos窗口
    go build -ldflags  "-a -s -w -H=windowsgui"
    start ./upx396/upx.exe --best fileSync.exe
    
# linux 编译
    go build -ldflags "-a -s -w"    
    ./upx396/upx --best fileSync

# 升级版本
 先删除 go.mod  go.sum  然后执行go  mod init fileSync go mod tidy 然后编译


    
    
