// The configuration of the app, which `mygo dev` and `mygo build` evaluate with
// Bun.
export default () => ({
  name: "Ringo",
  identifier: "io.github.lyjw131.ringo",
  slug: "ringo",
  version: "0.1.0",
  out: "build",
  resources: ["LICENSE"],
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
