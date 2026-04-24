import {LitElement, html, css} from 'https://cdn.jsdelivr.net/npm/lit@3/+esm';
import {unsafeHTML} from 'https://cdn.jsdelivr.net/npm/lit@3/directives/unsafe-html.js/+esm';
import {marked} from 'https://cdn.jsdelivr.net/npm/marked@latest/+esm';
import DOMPurify from 'https://cdn.jsdelivr.net/npm/dompurify@latest/+esm';
import hljs from 'https://cdn.jsdelivr.net/npm/highlight.js@latest/+esm';

// Configure marked to use highlight.js for fenced code blocks
marked.setOptions({
  highlight(code, lang) {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(code, {language: lang}).value;
    }
    return hljs.highlightAuto(code).value;
  },
});

class ChatMessage extends LitElement {
  static properties = {
    role: {type: String},
    content: {type: String},
    personaName: {type: String, attribute: 'persona-name'},
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
      word-break: break-word;
    }
    .message.user {
      background: #e0e7ff;
      color: #1e1b4b;
      margin-left: auto;
      border-bottom-right-radius: 2px;
      white-space: pre-wrap;
    }
    .message.assistant {
      background: #f3f4f6;
      color: #111827;
      margin-right: auto;
      border-bottom-left-radius: 2px;
    }
    .message.system {
      background: #fef3c7;
      color: #92400e;
      text-align: center;
      max-width: 100%;
      font-style: italic;
      font-size: 0.9rem;
      padding: 0.5rem 1rem;
      border-radius: 4px;
    }
    .role-label {
      font-size: 0.75rem;
      font-weight: 600;
      text-transform: uppercase;
      margin-bottom: 0.25rem;
      opacity: 0.7;
    }
    /* Markdown content styles for assistant messages */
    .message-body h1, .message-body h2, .message-body h3,
    .message-body h4, .message-body h5, .message-body h6 {
      margin: 0.5em 0 0.25em;
      line-height: 1.3;
    }
    .message-body h1 { font-size: 1.4em; }
    .message-body h2 { font-size: 1.2em; }
    .message-body h3 { font-size: 1.1em; }
    .message-body p {
      margin: 0.4em 0;
    }
    .message-body pre {
      background: #1e1e2e;
      color: #cdd6f4;
      padding: 0.75rem;
      border-radius: 6px;
      overflow-x: auto;
      font-size: 0.875rem;
      line-height: 1.4;
    }
    .message-body code {
      font-family: 'Fira Code', 'Consolas', monospace;
      font-size: 0.9em;
    }
    .message-body :not(pre) > code {
      background: #e5e7eb;
      padding: 0.15em 0.35em;
      border-radius: 3px;
    }
    .message-body ul, .message-body ol {
      margin: 0.4em 0;
      padding-left: 1.5em;
    }
    .message-body blockquote {
      border-left: 3px solid #9ca3af;
      margin: 0.4em 0;
      padding: 0.25em 0.75em;
      color: #6b7280;
    }
    .message-body a {
      color: #2563eb;
      text-decoration: underline;
    }
    .message-body table {
      border-collapse: collapse;
      margin: 0.4em 0;
    }
    .message-body th, .message-body td {
      border: 1px solid #d1d5db;
      padding: 0.35em 0.75em;
    }
    .message-body th {
      background: #f3f4f6;
    }
  `;

  constructor() {
    super();
    this.role = 'user';
    this.content = '';
    this.personaName = '';
  }

  get displayRole() {
    if (this.role === 'assistant' && this.personaName) {
      return this.personaName;
    }
    return this.role;
  }

  _renderContent() {
    if (this.role === 'assistant') {
      const rawHtml = marked.parse(this.content || '');
      const safeHtml = DOMPurify.sanitize(rawHtml);
      return html`<div class="message-body">${unsafeHTML(safeHtml)}</div>`;
    }
    if (this.role === 'system') {
      return html`<div class="message-body">${this.content}</div>`;
    }
    // user messages: plain text
    return html`<div class="message-body">${this.content}</div>`;
  }

  render() {
    if (this.role === 'system') {
      return html`
        <div class="message system">
          ${this._renderContent()}
        </div>
      `;
    }
    return html`
      <div class="message ${this.role}">
        <div class="role-label">${this.displayRole}</div>
        ${this._renderContent()}
      </div>
    `;
  }
}

customElements.define('chat-message', ChatMessage);
