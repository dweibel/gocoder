import {LitElement, html, css} from 'https://cdn.jsdelivr.net/npm/lit@3/+esm';

class LoadingIndicator extends LitElement {
  static properties = {
    active: {type: Boolean, reflect: true},
  };

  static styles = css`
    :host {
      display: block;
    }
    .indicator {
      display: none;
      align-items: center;
      gap: 0.5rem;
      padding: 0.5rem 0;
      color: #6b7280;
      font-size: 0.9rem;
    }
    :host([active]) .indicator {
      display: flex;
    }
    .dots {
      display: flex;
      gap: 4px;
    }
    .dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #6b7280;
      animation: bounce 1.4s infinite ease-in-out both;
    }
    .dot:nth-child(1) { animation-delay: -0.32s; }
    .dot:nth-child(2) { animation-delay: -0.16s; }
    .dot:nth-child(3) { animation-delay: 0s; }
    @keyframes bounce {
      0%, 80%, 100% { transform: scale(0); }
      40% { transform: scale(1); }
    }
  `;

  constructor() {
    super();
    this.active = false;
  }

  render() {
    return html`
      <div class="indicator" role="status" aria-label="Loading">
        <div class="dots">
          <span class="dot"></span>
          <span class="dot"></span>
          <span class="dot"></span>
        </div>
        <span>Thinking…</span>
      </div>
    `;
  }
}

customElements.define('loading-indicator', LoadingIndicator);
