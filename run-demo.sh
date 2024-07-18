#!/usr/bin/env bash
set -e
go build
patchelf --add-rpath $(nix eval --raw nixpkgs\#vulkan-loader.outPath)/lib ./wgpu-demo
patchelf --add-rpath $(nix eval --raw nixpkgs\#libGL.outPath)/lib ./wgpu-demo
patchelf --add-rpath $(nix eval --raw nixpkgs\#wayland.outPath)/lib ./wgpu-demo
# export VK_LAYER_PATH=$(nix eval --raw nixpkgs\#vulkan-validation-layers.outPath)/share/vulkan/explicit_layer.d
exec ./wgpu-demo
