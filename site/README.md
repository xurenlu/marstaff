# marstaff.me 官网

Marstaff 项目官网静态页面，引导用户前往 GitHub / Gitee 获取源码。

## 本地预览

```bash
# 使用 Python 简单起一个静态服务
cd site && python3 -m http.server 8080
# 访问 http://localhost:8080
```

或直接用浏览器打开 `index.html`。

## 部署

将 `site/` 目录下的 `index.html` 与 `css/` 部署到任意静态托管（如 GitHub Pages、Vercel、Netlify、阿里云 OSS 等）即可。

## 链接说明

- **GitHub**: https://github.com/rocky/marstaff
- **Gitee**: https://gitee.com/rocky/marstaff（若仓库路径不同，请修改 `index.html` 中的链接）
