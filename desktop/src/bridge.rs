//! Starting and feeding the `bub desktop` bridge.
//!
//! The UI holds no Bub logic. It spawns the bridge as a child process and
//! speaks newline-delimited JSON with it over stdio; this module owns the
//! spawn, the two reader threads, and the channel those threads feed.

use std::io::{BufRead, BufReader};
use std::process::{Child, ChildStdin, Command, Stdio};

/// A running bridge: the child, its stdin, and its parsed stdout lines.
pub(crate) struct Bridge {
    pub(crate) child: Child,
    pub(crate) stdin: ChildStdin,
    pub(crate) lines: smol::channel::Receiver<String>,
}

/// Start `bub desktop` and stream its stdout lines back to the UI thread.
///
/// Stdout is read on its own thread and handed over as an async channel so the
/// render loop can await lines without blocking. Stderr is forwarded to the
/// terminal instead, which keeps bridge tracebacks visible during development
/// without polluting the protocol stream.
pub(crate) fn spawn() -> Result<Bridge, String> {
    let mut last_error = String::from("no bridge command configured");

    for candidate in candidates() {
        let (program, args) = candidate.split_first().expect("candidate is non-empty");
        let spawned = Command::new(program)
            .args(args)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn();

        match spawned {
            Ok(mut child) => {
                let stdin = child.stdin.take().expect("child stdin");
                let stdout = child.stdout.take().expect("child stdout");
                let stderr = child.stderr.take().expect("child stderr");

                let (tx, lines) = smol::channel::unbounded::<String>();
                std::thread::spawn(move || {
                    for line in BufReader::new(stdout).lines() {
                        match line {
                            Ok(line) => {
                                if tx.send_blocking(line).is_err() {
                                    break;
                                }
                            }
                            Err(_) => break,
                        }
                    }
                });
                std::thread::spawn(move || {
                    for line in BufReader::new(stderr).lines().map_while(Result::ok) {
                        eprintln!("[bub bridge] {line}");
                    }
                });

                return Ok(Bridge {
                    child,
                    stdin,
                    lines,
                });
            }
            Err(err) => last_error = format!("{}: {err}", candidate.join(" ")),
        }
    }

    Err(last_error)
}

/// Bridge launch candidates, most specific first.
fn candidates() -> Vec<Vec<String>> {
    if let Ok(command) = std::env::var("BUB_DESKTOP_CMD") {
        let parts: Vec<String> = command.split_whitespace().map(str::to_string).collect();
        if !parts.is_empty() {
            return vec![parts];
        }
    }
    vec![
        vec!["bub".to_string(), "desktop".to_string()],
        vec![
            "uv".to_string(),
            "run".to_string(),
            "bub".to_string(),
            "desktop".to_string(),
        ],
    ]
}
