# 发布指南 — mcache-py

## 一次性准备

### 1. 注册 PyPI / TestPyPI 账号

- TestPyPI：https://test.pypi.org/account/register/（用于演练）
- PyPI：https://pypi.org/account/register/（正式）

### 2. 创建 API Token

在 https://pypi.org/manage/account/token/ 创建 token。Scope 选 "Entire account"（首次）或限定到 `mcache-py` 项目。

### 3. 配置认证（任选其一）

**方式 A：`~/.pypirc`（本地手工发布）**

```ini
[pypi]
username = __token__
password = pypi-AgEIcHl...

[testpypi]
repository = https://test.pypi.org/legacy/
username = __token__
password = pypi-AgENdGVz...
```

权限：`chmod 600 ~/.pypirc`（Linux/Mac）。Windows 权限默认即可。

**方式 B：GitHub Actions Trusted Publishing（推荐 — 无需 token）**

1. 仓库设置：Settings → Environments → 新建 `pypi` 和 `testpypi`
2. PyPI 后台：https://pypi.org/manage/project/mcache-py/settings/publishing/
   - Owner: `atoncooper`
   - Repository: `mcache`
   - Workflow: `python-publish.yml`
   - Environment: `pypi`
3. 同样在 TestPyPI 配置 `testpypi` environment
4. 之后 GitHub Actions 走 OIDC，零密钥发布

## 本地手工发布

### TestPyPI（演练）

```bash
cd sdk/python
./scripts/publish.sh test
```

或 Windows：

```powershell
cd sdk/python
.\scripts\publish.ps1 test
```

测试安装：
```bash
pip install --index-url https://test.pypi.org/simple/ mcache-py
python -c "from mcache import Client; print(Client)"
```

### PyPI（正式）

```bash
cd sdk/python
./scripts/publish.sh
```

会要求输入 `yes` 二次确认。

## CI/CD 自动发布（推荐）

仓库已配置 `.github/workflows/python-publish.yml`。

### 触发方式

| 方式 | 触发条件 | 目标 |
|------|---------|------|
| Tag push | `git tag py-v1.2.3 && git push origin py-v1.2.3` | PyPI |
| 手动 | Actions → Run workflow → testpypi | TestPyPI |
| 手动 | Actions → Run workflow → pypi | PyPI |

### 版本发布流程

```bash
# 1. 改版本号
#    sdk/python/pyproject.toml: version = "1.1.0"
#    sdk/python/mcache/__init__.py: __version__ = "1.1.0"

# 2. 更新 CHANGELOG
#    sdk/python/CHANGELOG.md 添加新版本条目

# 3. 提交
git add sdk/python/
git commit -m "release: mcache-py v1.1.0"

# 4. 打 tag 并推送（触发 GitHub Action 自动发布）
git tag py-v1.1.0
git push origin master --tags
```

## 检查清单

发布前确认：

- [ ] `pyproject.toml` 版本号已更新
- [ ] `mcache/__init__.py` 的 `__version__` 一致
- [ ] `CHANGELOG.md` 已添加新版本条目
- [ ] 本地能成功 `python -m build`
- [ ] 本地能通过 `python -m twine check dist/*`
- [ ] 单元测试全部通过
- [ ] README 中的示例代码可运行

## 常见问题

### `400 File already exists`
PyPI 不允许同一版本号重传。必须 bump version 后重新发布。

### `name conflict`
`mcache` 已被占用，本项目用的是 `mcache-py`。如需换名，修改 `pyproject.toml` 的 `name` 字段。

### Wheel 中缺少子模块
检查 `pyproject.toml` 的 `[tool.setuptools.packages.find]` 配置和 `MANIFEST.in`。

### `py.typed` 未生效
确认 `mcache/py.typed` 文件存在且 `package-data` 已包含。下游工具如 mypy / pyright 才会读取类型。

### Token 泄露应对
1. 立即在 https://pypi.org/manage/account/token/ 撤销 token
2. 创建新 token
3. 更新 `~/.pypirc` 或 GitHub Secrets
