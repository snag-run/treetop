# Changelog

## [0.3.0](https://github.com/snag-run/treetop/compare/v0.2.0...v0.3.0) (2026-06-27)


### Features

* clickable OSC 8 links for PR numbers and check runs ([#72](https://github.com/snag-run/treetop/issues/72)) ([d93dd90](https://github.com/snag-run/treetop/commit/d93dd904d06dc431e0a0e5e10ec1d3158e96ac04))
* credit snag.run in README and watch-mode title ([#74](https://github.com/snag-run/treetop/issues/74)) ([ad5b86c](https://github.com/snag-run/treetop/commit/ad5b86c2cfbd251a031ca4c76b5daf03e57bedb5))
* PR number in its own column, colored by review state ([#68](https://github.com/snag-run/treetop/issues/68)) ([2fe7ef8](https://github.com/snag-run/treetop/commit/2fe7ef82858d65f34243706d13b5194b28f154f5))


### Bug Fixes

* dedup --checks rows so a closed/reopened PR doesn't double them ([#64](https://github.com/snag-run/treetop/issues/64)) ([f8b480f](https://github.com/snag-run/treetop/commit/f8b480f055da097373d5455bc17ee04ade6270ce)), closes [#63](https://github.com/snag-run/treetop/issues/63)

## [0.2.0](https://github.com/snag-run/treetop/compare/v0.1.1...v0.2.0) (2026-06-27)


### Features

* add --checks to expand per-check CI rows under each worktree ([#56](https://github.com/snag-run/treetop/issues/56)) ([181d010](https://github.com/snag-run/treetop/commit/181d010d0746f33c5a9eb0842543c490ae955a3e))
* add --depth flag to scan nested repo layouts ([#50](https://github.com/snag-run/treetop/issues/50)) ([57a29d5](https://github.com/snag-run/treetop/commit/57a29d5349f7fa428b409b48bdb191323484a2c9))
* add --pr glyph showing PR check status per worktree ([#54](https://github.com/snag-run/treetop/issues/54)) ([35d017c](https://github.com/snag-run/treetop/commit/35d017cee4f6838ffe8cf1f6782aac32136e308f))
* add --version flag ([#27](https://github.com/snag-run/treetop/issues/27)) ([2bdc781](https://github.com/snag-run/treetop/commit/2bdc7818dc61c2668dcd250262f9ae1f6c7e2fa2))
* add a blank line between projects in the table ([#53](https://github.com/snag-run/treetop/issues/53)) ([8c449c8](https://github.com/snag-run/treetop/commit/8c449c87786141ec2ec08f01886e4fd2b8b65fcb))
* bound the walkNewest working-tree walk ([#44](https://github.com/snag-run/treetop/issues/44)) ([3bffa47](https://github.com/snag-run/treetop/commit/3bffa47f4826443e25ab5a28a137abdd57f56e1b))
* distinguish filtered-to-empty from no worktrees in one-shot output ([#46](https://github.com/snag-run/treetop/issues/46)) ([9ac62ed](https://github.com/snag-run/treetop/commit/9ac62edaa9c7badd7c8fcd978a7aa620a292f268))
* flag stale data in the watch-mode header ([#52](https://github.com/snag-run/treetop/issues/52)) ([dbd571b](https://github.com/snag-run/treetop/commit/dbd571bf5b38773998fc959baf150bfddbf94102))
* note unsupported session detection in one-shot output ([#47](https://github.com/snag-run/treetop/issues/47)) ([e23062b](https://github.com/snag-run/treetop/commit/e23062b26504e93adedcfc5196214a38f198ab9b))
* restore the terminal on SIGHUP/SIGQUIT in watch mode ([#51](https://github.com/snag-run/treetop/issues/51)) ([be5875d](https://github.com/snag-run/treetop/commit/be5875d0ca40930f2cda790073d9467b3d11c3a7))
* show the open PR number on each worktree row ([#60](https://github.com/snag-run/treetop/issues/60)) ([daa19ff](https://github.com/snag-run/treetop/commit/daa19ff3e218efb9de82c1ac30754c144aec5d83))


### Bug Fixes

* bound git rev-parse and worktree-list calls with a timeout ([#41](https://github.com/snag-run/treetop/issues/41)) ([f7cab63](https://github.com/snag-run/treetop/commit/f7cab633a8fe86ea306fb37d0be75b3d23d235c7))
* error out when no scan root can be resolved ([#43](https://github.com/snag-run/treetop/issues/43)) ([ac2ca04](https://github.com/snag-run/treetop/commit/ac2ca04e1ed2b8a3dea39f256850e81d9ab854e8)), closes [#33](https://github.com/snag-run/treetop/issues/33)
* evict stale entries from the edit cache ([#42](https://github.com/snag-run/treetop/issues/42)) ([0422363](https://github.com/snag-run/treetop/commit/0422363dbf850b6cb163d6127de77625eb3d4fac))
* require consent before install.sh edits the global gitignore ([#48](https://github.com/snag-run/treetop/issues/48)) ([33e9dc5](https://github.com/snag-run/treetop/commit/33e9dc586c58693805b29ec2aa0b64f06b117c3d))
* verify a marker PID is still an agent before honoring it ([#49](https://github.com/snag-run/treetop/issues/49)) ([f0af503](https://github.com/snag-run/treetop/commit/f0af503adf9f04c8ab1ed458297675c4fd86318d))
* warn when a --root is unreadable instead of skipping silently ([#45](https://github.com/snag-run/treetop/issues/45)) ([559c7d6](https://github.com/snag-run/treetop/commit/559c7d6096ed5cc2b1cf5b94006c41b72a2c00e9))

## [0.1.1](https://github.com/snag-run/treetop/compare/v0.1.0...v0.1.1) (2026-06-27)


### Bug Fixes

* prevent code execution and terminal injection from scanned repos ([#23](https://github.com/snag-run/treetop/issues/23)) ([733b5ce](https://github.com/snag-run/treetop/commit/733b5ce7ba7665f40ecfd4108da57c2cbcdbc714))

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
