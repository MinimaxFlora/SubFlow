<div align="center">
<br>
<img width="200" src="./public/favicon.svg" alt="SubFlow">
<br>
<br>
<h2 align="center">SubFlow Web</h2>
</div>

<p align="center" color="#6a737d">
Frontend for SubFlow, a self-hosted subscription management platform.
</p>

## Upstream attribution

This directory is a modified copy of the frontend of **Sub-Store**, originally
created by [Peng-YM](https://github.com/Peng-YM) and now maintained by the
[sub-store-org](https://github.com/sub-store-org/Sub-Store) organization.

- Upstream project: <https://github.com/sub-store-org/Sub-Store>
- Upstream license: **GNU General Public License v3.0** (see [`LICENSE`](./LICENSE))
- Baseline version imported here: Sub-Store frontend `v2.34.1`

SubFlow is not affiliated with or endorsed by the Sub-Store project. The
Sub-Store name and original artwork remain the property of their respective
owners and are used here only to describe the origin of this code.

**Modifications applied in this repository:**

- Rebranded the user-visible product name and metadata from `Sub-Store` to
  `SubFlow` (locales, manifests, window title, PWA metadata).
- Replaced the application icon and PWA assets with original SubFlow artwork.
- Pinned the API base URL to the same origin so the embedded Go backend serves
  the API under a single host.
- Localized additional interface strings.

Because the upstream is licensed under the GPL-3.0, this directory and any
derivative of it **must remain under the GPL-3.0**. The full license text is in
[`LICENSE`](./LICENSE), and the complete corresponding source for this
modified version is this repository. See the repository-level
[`NOTICE`](../NOTICE) for the combined-work licensing of SubFlow.

## Development

#### Guidelines

Commit message follows [@commitlint/config-angular](https://github.com/conventional-changelog/commitlint/tree/master/@commitlint/config-angular)

#### Toolchain
- pnpm v11.0.9 via Corepack
- Vite v3
- Vue v3
- Pinia v2
- TypeScript v4.9

#### Font Awesome Icon
This project uses [Font Awesome](https://fontawesome.com/icons) icons.

#### Start
```bash
# enable the pinned package manager
corepack enable

# install dependencies
pnpm i

# run the development server
pnpm dev

# build for production
pnpm build

# preview the production build
pnpm preview
```

## License

This project is distributed under the **GNU General Public License v3.0**.

## Acknowledgements

- [Sub-Store](https://github.com/sub-store-org/Sub-Store) and [Peng-YM](https://github.com/Peng-YM) for the original frontend.
- Special thanks to @KOP-XIAO for the resource-parser. Please give a
  [star](https://github.com/KOP-XIAO/QuantumultX).
- Special thanks to @Orz-3 and @58xinian for their icons.
