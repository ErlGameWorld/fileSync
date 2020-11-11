# fileSync 同步文件更改相关

# go windows下程序运行隐藏dos窗口
    go build -ldflags  "-a -s -w -H=windowsgui"
    start ./upx396/upx.exe --best fileSync.exe
    
# linux 编译
    go build -ldflags "-a -s -w"    
    ./upx396/upx --best fileSync


    
    
