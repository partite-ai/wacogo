JS Engine wrapper wrapping https://github.com/golemcloud/wasm-rquickjs

Build using 

`PATH="$PATH:/Users/mpoindexter/.cargo/bin" wasm-rquickjs generate-wrapper-crate --wit . --js wrapper.js --output /tmp/wrapper-crate`

`PATH="$PATH:$(brew --prefix rustup)/bin" cargo build --target wasm32-wasip2 --release --no-default-features --features "lite,node-http,crypto,zlib"`

then copy the resulting component.wasm file to wrapper.wasm