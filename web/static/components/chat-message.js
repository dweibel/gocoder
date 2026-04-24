import {LitElement, html, css} from 'https://cdn.jsdelivr.net/npm/lit@3/+esm';

class ChatMessage extends LitElement {
  static properties = {
    role: {type: String},
    content: {type: String},
  };

  static styles = css`
    :host {
      display: block;
      margin-bottom: 0.75rem;
    }
    .message {
      padding: 0.75rem 1rem;
      border-radius: 8px;
      max-width: 80%;
      line-height: 1.5;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .message.user {
      background: #e0e7ff;
      color: #1e1b4b;
      margin-left: auto;
      border-bottom-right-radius: 2px;
    }
    .message.assistant {
      background: #f3f4f6;
      color: #111827;
      margin-right: auto;
      border-bottom-left-radius: 2px;
    }
    .role-label {
      font-size: 0.75rem;
      font-weight: 600;
      text-transform: uppercase;
      margin-bottom: 0.25rem;
      opacity: 0.7;
    }
  `;

  constructor() {
    super();
    this.role = 'user';
    this.content = '';
  }

  render() {
    return html`
      <div class="message ${this.role}">
        <div class="role-label">${this.role}</div>
        <div class="message-body">${this.content}</div>
      </div>
    `;
  }
}

customElements.define('chat-message', ChatMessage);
