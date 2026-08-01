-------0.0.43------
- Security: Blocked telemetry/ads connections to external servers
- Security: Dynamically generate unique local Root CA certificate and private key per installation
-------0.0.42------
- Fixed grep or read blocking for long periods @liorxuan
-------0.0.41------
- Fixed memory leak issue that could cause abnormal memory usage
- Supported Russian and expanded translation scope
- Fixed qwen-3.8-max interruption issue (mimo should also belong to the same type of issue)
- Fixed issue where claude model might fail to recognize images @GGHansome
- Supported custom OpenAI endpoint @Sxuan-Coder
- WebSearch connected to Baidu search, DuckDuckGo as fallback @Yang Chao
- Fixed some compatibility issues @kael-odin 
- Fixed some display issues on Windows @philau2512

🔔 How to let AI pull model configs automatically? (The following prompt can be used by replacing address and key with yours)
My model config is in ~/.cursor-local-assistant-v2/config.yaml, my API address is: https://xxx and key is xxx, help me pull all model configs into it without affecting existing models, pulling based on the standard models endpoint.