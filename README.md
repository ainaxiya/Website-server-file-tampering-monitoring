# Welcome to Website-server-file-tampering-monitoring

这个是golang编写的web服务器文件篡改或变动监控，可推送消息警告。

使用方法：
运行时先配置data文件夹里面的config.json配置文件，
directories 这是配置需要监控的文件夹路径可多个，
exclude 这是排除掉的文件或文件夹，这下面的文件将不被监控，可以*.html这样通配后缀。
编译一下它 monitoringserver.go
go build -o yourname monitoringserver.go
或者
go run monitoringserver.go
就OK了 20分钟扫描一次 
运行后会扫描监控的所有文件并且保存hash码
hashdb.json 这个是保存监控的所有文件hash码的数据json，
webmonitor.log这个是日志文件，监控的文件有任何变动都会保存进日志。
