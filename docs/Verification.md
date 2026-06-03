# 云端 - 网关认证逻辑

## 第一版

后端在配置文件里登记多个device，包含sn和token。前端在配置文件里登记一个sn和一个token。中间用sn和token pair进行握手认证。

## 正式版

- 客户端用cli生成sn(UUID)，
- 服务端预设一次性token
- 客户端填入token，握手的时候携带sn和token
- token握手后作废
- 服务端存储SN和生成的随机长密码哈希，把随机长密码返回前端并持久化
- 以后都用sn和随机长密码对握手
