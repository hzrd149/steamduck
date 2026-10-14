{
  description = "Steamduck development tools";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              go_1_26
              nodejs_24
              just
              git
            ];

            # Kwakore owns the webview; the Steamduck broker builds without cgo.
            CGO_ENABLED = "0";
            # Use the pinned compiler rather than downloading one implicitly.
            GOTOOLCHAIN = "local";
          };
        });
    };
}
