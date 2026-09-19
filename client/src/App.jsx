import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";

const API =
  import.meta.env.VITE_API_URL || `http://${window.location.hostname}:8080`;
const WS_BASE =
  import.meta.env.VITE_WS_URL ||
  `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.hostname}:8080`;

function getPollIdFromPath() {
  const match = window.location.pathname.match(/^\/poll\/([a-f0-9]{24})\/?$/i);
  return match ? match[1] : null;
}

function shareUrlForPoll(pollId) {
  return `${window.location.origin}/poll/${pollId}`;
}

function copyText(text) {
  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text);
  }

  const input = document.createElement("input");
  input.value = text;
  document.body.appendChild(input);
  input.select();
  document.execCommand("copy");
  document.body.removeChild(input);
  return Promise.resolve();
}

function PollCard({ poll, onVote, showShare = true }) {
  const [copied, setCopied] = useState(false);
  const shareUrl = useMemo(() => shareUrlForPoll(poll.id), [poll.id]);

  const copyShareLink = async () => {
    await copyText(shareUrl);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <article className="poll">
      <div className="poll-top">
        <span>Poll #{poll.id}</span>
        <strong>{poll.totalVotes} votes</strong>
      </div>

      <h2>{poll.question}</h2>

      <div className="options">
        {poll.options.map((option, index) => {
          const count = poll.votes?.[index] || 0;
          const percent = poll.totalVotes
            ? Math.round((count / poll.totalVotes) * 100)
            : 0;

          return (
            <button
              className="option"
              key={`${poll.id}-${index}`}
              onClick={() => onVote(poll.id, index)}
              type="button"
            >
              <span className="option-line">
                <b>{option}</b>
                <span className="count">
                  {count} - {percent}%
                </span>
              </span>
              <span className="bar">
                <i style={{ width: `${percent}%` }} />
              </span>
            </button>
          );
        })}
      </div>

      {showShare && (
        <div className="share-row">
          <a href={`/poll/${poll.id}`}>Open voting link</a>
          <button className="ghost" type="button" onClick={copyShareLink}>
            {copied ? "Copied" : "Copy link"}
          </button>
        </div>
      )}
    </article>
  );
}

function AuthPanel({ auth, onAuth }) {
  const [mode, setMode] = useState("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async event => {
    event.preventDefault();
    setBusy(true);

    try {
      const response = await fetch(`${API}/api/auth/${mode}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Authentication failed");

      localStorage.setItem("pulsepoll_auth", JSON.stringify(data));
      onAuth(data);
    } catch (error) {
      alert(error.message);
    } finally {
      setBusy(false);
    }
  };

  if (auth?.token) {
    return (
      <section className="auth-panel">
        <span>Signed in as {auth.user.email}</span>
        <button
          className="ghost"
          type="button"
          onClick={() => {
            localStorage.removeItem("pulsepoll_auth");
            onAuth(null);
          }}
        >
          Sign out
        </button>
      </section>
    );
  }

  return (
    <section className="auth-panel">
      <form onSubmit={submit}>
        <input
          value={email}
          onChange={event => setEmail(event.target.value)}
          placeholder="Email"
          type="email"
          required
        />
        <input
          value={password}
          onChange={event => setPassword(event.target.value)}
          placeholder="Password"
          type="password"
          minLength={6}
          required
        />
        <button type="submit" disabled={busy}>
          {busy ? "Please wait..." : mode === "login" ? "Log in" : "Sign up"}
        </button>
        <button
          className="ghost"
          type="button"
          onClick={() => setMode(mode === "login" ? "signup" : "login")}
        >
          {mode === "login" ? "Need account?" : "Have account?"}
        </button>
      </form>
    </section>
  );
}

function CreatePoll({ auth, onCreated }) {
  const [question, setQuestion] = useState("");
  const [options, setOptions] = useState(["", ""]);
  const [submitting, setSubmitting] = useState(false);

  const updateOption = (index, value) => {
    setOptions(current =>
      current.map((option, i) => (i === index ? value : option))
    );
  };

  const addOption = () => {
    if (options.length < 8) setOptions(current => [...current, ""]);
  };

  const removeOption = index => {
    if (options.length <= 2) return;
    setOptions(current => current.filter((_, i) => i !== index));
  };

  const submit = async event => {
    event.preventDefault();

    const cleanOptions = options.map(option => option.trim()).filter(Boolean);
    if (question.trim().length < 3 || cleanOptions.length < 2) return;

    setSubmitting(true);
    try {
      const response = await fetch(`${API}/api/polls`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${auth.token}`,
        },
        body: JSON.stringify({
          question: question.trim(),
          options: cleanOptions,
        }),
      });

      const poll = await response.json();
      if (!response.ok) throw new Error(poll.error || "Could not create poll");
      setQuestion("");
      setOptions(["", ""]);
      onCreated(poll);
    } catch (error) {
      alert(error.message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="creator">
      <div className="section-heading">
        <div>
          <span className="eyebrow">NEW QUESTION</span>
          <h2>Create a poll</h2>
        </div>
      </div>

      <form onSubmit={submit}>
        <input
          value={question}
          onChange={event => setQuestion(event.target.value)}
          placeholder="Ask a question..."
          minLength={3}
          required
        />

        <div className="option-inputs">
          {options.map((option, index) => (
            <div className="input-row" key={index}>
              <input
                value={option}
                onChange={event => updateOption(index, event.target.value)}
                placeholder={`Option ${index + 1}`}
                required
              />
              {options.length > 2 && (
                <button
                  className="remove"
                  type="button"
                  onClick={() => removeOption(index)}
                  aria-label={`Remove option ${index + 1}`}
                >
                  x
                </button>
              )}
            </div>
          ))}
        </div>

        <div className="form-actions">
          <button className="secondary" type="button" onClick={addOption}>
            Add option
          </button>
          <button type="submit" disabled={submitting}>
            {submitting ? "Creating..." : "Create poll"}
          </button>
        </div>
      </form>
    </section>
  );
}

function useRealtimePoll(pollId, onMissingPoll) {
  const [poll, setPoll] = useState(null);
  const [status, setStatus] = useState("Loading poll...");
  const socketRef = useRef(null);

  const applyRealtimeUpdate = useCallback(data => {
    setPoll(current =>
      current
        ? {
            ...current,
            votes: data.votes,
            totalVotes: data.totalVotes,
          }
        : current
    );
  }, []);

  const connectPoll = useCallback(() => {
    if (!pollId) return;
    const existing = socketRef.current;
    if (existing && existing.readyState < 2) return;

    const socket = new WebSocket(`${WS_BASE}/ws/${pollId}`);
    socketRef.current = socket;

    socket.onmessage = event => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === "snapshot" || data.type === "vote_update") {
          applyRealtimeUpdate(data);
        }
      } catch {
        // Ignore malformed messages.
      }
    };

    socket.onclose = () => {
      if (socketRef.current === socket) socketRef.current = null;
      window.setTimeout(connectPoll, 1500);
    };
  }, [applyRealtimeUpdate, pollId]);

  const loadPoll = useCallback(async () => {
    if (!pollId) return;

    try {
      const response = await fetch(`${API}/api/polls/${pollId}`);
      if (response.status === 404) {
        onMissingPoll?.();
        throw new Error("Poll not found");
      }
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Could not load poll");
      setPoll(data);
      setStatus("Live results");
      connectPoll();
    } catch (error) {
      setStatus(`Backend unavailable: ${error.message}`);
    }
  }, [connectPoll, onMissingPoll, pollId]);

  useEffect(() => {
    loadPoll();

    return () => {
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [loadPoll]);

  const vote = async optionIndex => {
    if (!pollId) return;

    try {
      const response = await fetch(`${API}/api/polls/${pollId}/vote`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ optionIndex }),
      });

      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Could not vote");
      applyRealtimeUpdate(data);
    } catch (error) {
      alert(error.message);
    }
  };

  return { poll, status, vote };
}

function PollPage({ pollId }) {
  const [copied, setCopied] = useState(false);
  const { poll, status, vote } = useRealtimePoll(pollId);
  const shareUrl = useMemo(() => shareUrlForPoll(pollId), [pollId]);

  const copyShareLink = async () => {
    await copyText(shareUrl);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <main className="shell narrow">
      <header>
        <div>
          <span className="eyebrow">SHARED POLL</span>
          <h1>PulsePoll</h1>
          <p>Vote from this link and watch the results update live.</p>
        </div>

        <a className="button-link secondary" href="/">
          Create poll
        </a>
      </header>

      <div className="status">
        <span className="live-dot" />
        {status}
      </div>

      {poll ? (
        <>
          <PollCard
            poll={poll}
            onVote={(_, optionIndex) => vote(optionIndex)}
            showShare={false}
          />
          <div className="share-panel">
            <input value={shareUrl} readOnly aria-label="Share link" />
            <button type="button" onClick={copyShareLink}>
              {copied ? "Copied" : "Copy link"}
            </button>
          </div>
        </>
      ) : (
        <section className="empty">No poll loaded yet.</section>
      )}
    </main>
  );
}

function HomePage() {
  const [polls, setPolls] = useState([]);
  const [status, setStatus] = useState("Loading polls...");
  const [auth, setAuth] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem("pulsepoll_auth"));
    } catch {
      return null;
    }
  });

  const loadPolls = useCallback(async () => {
    try {
      const response = await fetch(`${API}/api/polls`);
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Could not load polls");
      setPolls(data);
      setStatus(`${data.length} poll${data.length === 1 ? "" : "s"} ready`);
    } catch (error) {
      setStatus(`Backend unavailable: ${error.message}`);
    }
  }, []);

  useEffect(() => {
    loadPolls();
  }, [loadPolls]);

  const openCreatedPoll = poll => {
    window.history.pushState({}, "", `/poll/${poll.id}`);
    window.dispatchEvent(new PopStateEvent("popstate"));
  };

  return (
    <main className="shell">
      <header>
        <div>
          <span className="eyebrow">LIVE GO POLLS</span>
          <h1>PulsePoll</h1>
          <p>Create a poll, share its link, and let people vote in real time.</p>
        </div>
      </header>

      <AuthPanel auth={auth} onAuth={setAuth} />

      {auth?.token ? (
        <CreatePoll auth={auth} onCreated={openCreatedPoll} />
      ) : (
        <section className="empty">Log in or sign up to create a poll.</section>
      )}

      <div className="status">
        <span className="live-dot" />
        {status}
      </div>

      <section className="grid">
        {polls.map(poll => (
          <PollCard
            key={poll.id}
            poll={poll}
            onVote={() => {
              window.location.href = `/poll/${poll.id}`;
            }}
          />
        ))}
      </section>
    </main>
  );
}

export default function App() {
  const [pollId, setPollId] = useState(() => getPollIdFromPath());

  useEffect(() => {
    const syncRoute = () => setPollId(getPollIdFromPath());
    window.addEventListener("popstate", syncRoute);
    return () => window.removeEventListener("popstate", syncRoute);
  }, []);

  return pollId ? <PollPage pollId={pollId} /> : <HomePage />;
}
