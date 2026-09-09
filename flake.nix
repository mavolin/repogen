{
  inputs = {
    nixpkgs.url = "nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    utils,
  }:
    utils.lib.eachDefaultSystem (
      system: let
        pkgs = import nixpkgs {
          inherit system;
        };
      in {
        formatter = pkgs.alejandra;

        devShells.default = let
          go = pkgs.go;
        in pkgs.mkShell {
          packages = with pkgs; [
              go
              goreleaser
            ];

            shellHook = ''
              mkdir -p .sdk
              ln -sfn ${go} .sdk/go
            '';

            GOTOOLCHAIN = "local";
            GOROOT = "${go}/share/go";

          hardeningDisable = ["fortify"];
        };
      }
    );
}
