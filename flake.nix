{
  description = "Share Claude Code session history across machines, pick one in fzf, and resume it";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEach = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      ccreel = pkgs: pkgs.buildGoModule {
        pname = "ccreel";
        version = "0.1.0";
        src = ./.;
        # 依存は標準ライブラリだけなので vendor するものが無い
        vendorHash = null;
        subPackages = [ "cmd/ccreel" ];
        nativeBuildInputs = [ pkgs.makeWrapper ];
        # 外部コマンドとして呼ぶもの。タイマーから起動されると PATH が細いので補う。
        # 利用者の PATH にあるものを優先するため後ろに足す
        postInstall = ''
          wrapProgram $out/bin/ccreel --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.fzf pkgs.ripgrep pkgs.rsync pkgs.openssh ]}
          install -Dm644 skills/ccreel/SKILL.md $out/share/ccreel/skills/ccreel/SKILL.md
        '';
        # テストは rsync と sh で一時ディレクトリのハブを動かすので、サンドボックスの外（CI）で回す
        doCheck = false;
      };
    in
    {
      packages = forEach (pkgs: { default = ccreel pkgs; });
    };
}
