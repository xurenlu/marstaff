#!/usr/bin/env ruby
# 超简单的静态站点 HTTP 服务器
# 用法: ruby serve_site.rb
# 访问: http://localhost:8822

require 'webrick'

SITE_DIR = File.expand_path('site', __dir__)
PORT = 8899

unless Dir.exist?(SITE_DIR)
  puts "错误: site 目录不存在: #{SITE_DIR}"
  puts "请先创建 site 目录并放入静态文件"
  exit 1
end

server = WEBrick::HTTPServer.new(
  Port: PORT,
  DocumentRoot: SITE_DIR,
  BindAddress: '127.0.0.1'
)

trap('INT') { server.shutdown }
trap('TERM') { server.shutdown }

puts "静态站点服务已启动"
puts "访问地址: http://localhost:#{PORT}"
puts "站点目录: #{SITE_DIR}"
puts "按 Ctrl+C 停止"
puts "-" * 40

server.start
