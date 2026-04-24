import {LitElement, html, css} from 'https://cdn.jsdelivr.net/npm/lit@3/+esm';

class ChatInput extends LitElement {
  static properties = {
    sessionId: {type: String, attribute: 'session-id'},
    disabled: {type: Boolean, reflect: true},
    _value: {type: String, state: true},
  };

  static styles = css`
    :host {
      display: block;
      padding: 0.5rem 0;
    }
    .input-row {
      display: flex;
      gap: 0.5rem;
    }
    textarea {
      flex: 1;
      padding: 0.75rem;
      border: 1px solid #ccc;
      border-radius: 6px;
      font-family: inherit;
      font-size: 0.95rem;
      resize: none;
      min-height: 2.5rem;
      max-height: 8rem;
    }
    textarea:focus {
      outline: none;
      border-color: #4f46e5;
      box-shadow: 0 0 0 2px rgba(79,70,229,0.15);
    }
    textarea:disabled {
      background: #f3f4f6;
      cursor: not-allowed;
    }
    button {
      padding: 0.75rem 1.25rem;
      background: #4f46e5;
      color: #fff;
      border: none;
      border-radius: 6px;
      cursor: pointer;
      font-size: 0.95rem;
    }
    button:hover:not(:disabled) {
      background: #4338ca;
    }
    button:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }
  `;

  constructor() {
    super();
    this.sessionId = '';
    this.disabled = false;
    this._value = '';
  }

  render() {
    return html`
      <div class="input-row">
        <textarea
          placeholder="Type your message…"
          .value=${this._value}
          ?disabled=${this.disabled}
          @input=${this._onInput}
          @keydown=${this._onKeydown}
          rows="1"
          aria-label="Chat message input"
        ></textarea>
        <button ?disabled=${this.disabled || !this._value.trim()} @click=${this._send}>
          Send
        </button>
      </div>
    `;
  }

  _onInput(e) {
    this._value = e.target.value;
  }

  _onKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      this._send();
    }
  }

  _send() {
    const text = this._value.trim();
    if (!text || this.disabled) return;
    this.dispatchEvent(new CustomEvent('send-message', {
      detail: {content: text, sessionId: this.sessionId},
      bubbles: true,
      composed: true,
    }));
    this._value = '';
  }
}

customElements.define('chat-input', ChatInput);
