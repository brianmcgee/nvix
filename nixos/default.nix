{
  flake.nixosModules = {
    store = import ./store.nix;
  };
}
