# サブネット(sakuracloud_subnet)

---

ルータ(`sakuracloud_internet`)に追加可能なグローバルIPアドレスブロックを表すリソースです。  

### 設定例

```hcl
data sakuracloud_subnet "foobar" {
  # ルータのID
  internet_id = "${sakuracloud_internet.foobar.id}"

  # サブネットのインデックス
  index = 0
}
```

### パラメーター

|パラメーター         |必須  |名称                |初期値     |設定値                    |補足                                          |
|-------------------|:---:|--------------------|:--------:|------------------------|----------------------------------------------|
| `internet_id`     | ◯   | ルータID              | -        | 文字列                  | - |
| `index`           | ◯   | サブネットインデックス  | - | 数値 | - |
| `zone` | - | ゾーン | - | `is1a`<br />`is1b`<br />`tk1a`<br />`tk1v` | - |

### 属性

|属性名                | 名称                    | 補足                                        |
|---------------------|------------------------|--------------------------------------------|
| `id`                | スイッチID               | -                                          |
| `switch_id`         | スイッチID              | (内部的に)接続されているスイッチID              |
| `nw_address`        | ネットワークアドレス      | 割り当てられたグローバルIPのネットワークアドレス |
| `min_ipaddress`  | 最小IPアドレス           | 割り当てられたグローバルIPアドレスのうち、利用可能な先頭IPアドレス |
| `max_ipaddress`  | 最大IPアドレス           | 割り当てられたグローバルIPアドレスのうち、利用可能な最後尾IPアドレス |
| `ipaddresses`    | IPアドレスリスト         | 割り当てられたグローバルIPアドレスのうち、利用可能なIPアドレスのリスト |