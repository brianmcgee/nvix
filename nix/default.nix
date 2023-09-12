{inputs, ...}: {
  imports = [
    inputs.flake-root.flakeModule
    ./checks.nix
    ./devshell.nix
    ./dev
    ./docs.nix
    ./formatter.nix
    ./packages.nix
  ];
}
