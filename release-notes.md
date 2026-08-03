-------0.0.47-------
- Fixed Task/subagent bridge session binding by forwarding root conversation IDs and execution options
- Return explicit RunSSE errors when an active stream becomes unavailable instead of hanging silently
- Increased compaction reserve for large-context sessions

-------0.0.46------
- Integrated OpenSERP as the default web search engine with Google, Baidu, DuckDuckGo, and Yandex top-five results
- Added Yandex/Ecosia fallback handling and embedded OpenSERP binary packaging
- Added automated integration tests for search aggregation, fallback, error thresholds, and binary extraction

-------0.0.45------
- Redesigned model settings with multiple endpoint cards and endpoint-scoped model lists
- Split endpoint models into Active and Available card sections with Cursor visibility toggles

-------0.0.44------
- Translated CJK and Vietnamese strings to English across Go backend, Vue frontend UI, Taskfiles, and agent skills
- Audited API contracts and state keys with rollback checkpointing
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
