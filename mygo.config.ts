// The configuration of the app, which `mygo dev` and `mygo build` evaluate with
// Bun.
export default () => ({
  name: "Ringo",
  identifier: "io.github.lyjw131.ringo",
  slug: "ringo",
  version: "0.2.1",
  out: "build",
  resources: ["LICENSE", "THIRD_PARTY_NOTICES.md", "licenses/LGPL-2.1.txt"],
  macos: {
    icon: "icon/AppIcon.icon",
    minimumSystemVersion: "13.0",
  },
  linux: {
    comment: "A native Apple Music player",
    categories: ["AudioVideo", "Audio", "Player"],
    webview: false,
  },
});
