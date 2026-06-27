# Changelog

## 0.1.0 (2026-06-27)


### Features

* --projects collapsed view; drop --active/--inactive aliases ([eb30cb9](https://github.com/snag-run/treetop/commit/eb30cb9d547915877822df42910457cc564fe08c))
* add macOS support for live-session detection ([#15](https://github.com/snag-run/treetop/issues/15)) ([38723c2](https://github.com/snag-run/treetop/commit/38723c20d3072f5749220a38a62ad94fcf40bd6b))
* full-screen live dashboard for watch mode ([bbfa687](https://github.com/snag-run/treetop/commit/bbfa6879397f3a5f0417a44bec843ec58adc87db))
* in-use/open vocabulary and last-changed column ([ec2bf89](https://github.com/snag-run/treetop/commit/ec2bf89fbe88bf8e01b11b655d14fef4b30e2367))
* interactive filter box to type-to-filter projects in live mode ([#17](https://github.com/snag-run/treetop/issues/17)) ([c9191dc](https://github.com/snag-run/treetop/commit/c9191dc4d98fa0c02daf01f38a568150d73eacff))
* multi-project filtering with regex and grep-style -e ([#6](https://github.com/snag-run/treetop/issues/6)) ([ec4a712](https://github.com/snag-run/treetop/commit/ec4a71254c017e0dee6a0219360707534f6b6ab4))
* scrollable live dashboard (mouse wheel + keys) ([aa380c6](https://github.com/snag-run/treetop/commit/aa380c61f276f87d49928463232ce8e5658918a5)), closes [#1](https://github.com/snag-run/treetop/issues/1)
* split in-use vs file-edit indicators; detect subagents ([#7](https://github.com/snag-run/treetop/issues/7)) ([2c560d7](https://github.com/snag-run/treetop/commit/2c560d7e1ffaaf1facb6559bd8be8c920bcc5828))
* treetop — track git worktrees across projects ([a6bcbc9](https://github.com/snag-run/treetop/commit/a6bcbc9206b6a07e4112d463198c7f8175435985))


### Bug Fixes

* address macOS support review follow-ups ([#15](https://github.com/snag-run/treetop/issues/15)) ([#18](https://github.com/snag-run/treetop/issues/18)) ([422200e](https://github.com/snag-run/treetop/commit/422200e9f323e3b3d57dab1245d5656bbf9a9965))
* drain queued input before redrawing in live mode ([a86e236](https://github.com/snag-run/treetop/commit/a86e23625d0beba6223ae73dfad2accb2c9da79c))
* keep live mode responsive by polling off the input loop ([74649e2](https://github.com/snag-run/treetop/commit/74649e2a964f351160d53ac632edde18eb0cc739))
* keep live mode responsive by polling off the input loop ([7f0d282](https://github.com/snag-run/treetop/commit/7f0d2825fefb9139c7c216b1502da3a8eb73384a))
* set release-please initial-version to 0.1.0 ([#11](https://github.com/snag-run/treetop/issues/11)) ([a1ce089](https://github.com/snag-run/treetop/commit/a1ce08954e3851a6ff20084dda64d495b628e38c))
