# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0-rc2] - 2025-02-24

### Fixed

- 图片生成空响应：当 LLM 执行工具（如 generate_image）后未返回文本时，使用工具执行结果作为响应内容，避免「后端返回空响应」

## [1.0.0] - 2025-02-24

### Added

- 版本号管理：`Version` 变量、`/api/health` 返回 version、`X-Marstaff-Version` 响应头
- CHANGELOG.md 与 README 版本信息、变更记录链接
