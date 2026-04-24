import {LitElement, html, css} from 'https://cdn.jsdelivr.net/npm/lit@3/+esm';

class PersonaSwitcher extends LitElement {
  static properties = {
    currentPersona: {type: String},
    sessionId: {type: String},
    personas: {type: Array},
  };

  static styles = css`
    :host {
      display: inline-block;
    }
    .switcher {
      position: relative;
    }
    select {
      padding: 0.4rem 0.6rem;
      border: 1px solid #d1d5db;
      border-radius: 6px;
      background: #fff;
      font-size: 0.85rem;
      cursor: pointer;
      color: #374151;
    }
    select:focus {
      outline: 2px solid #6366f1;
      outline-offset: 1px;
    }
  `;

  constructor() {
    super();
    this.currentPersona = '';
    this.sessionId = '';
    this.personas = [];
  }

  connectedCallback() {
    super.connectedCallback();
    // Read data attributes from the host element
    if (this.dataset.sessionId) {
      this.sessionId = this.dataset.sessionId;
    }
    if (this.dataset.currentPersona) {
      this.currentPersona = this.dataset.currentPersona;
    }
    if (this.dataset.personas) {
      try {
        this.personas = JSON.parse(this.dataset.personas);
      } catch (_) {
        this.personas = [];
      }
    }
  }

  _handleChange(e) {
    const selected = e.target.value;
    if (selected && selected !== this.currentPersona) {
      this.dispatchEvent(new CustomEvent('persona-change', {
        detail: {persona: selected},
        bubbles: true,
        composed: true,
      }));
    }
  }

  render() {
    return html`
      <div class="switcher">
        <label>
          <span class="sr-only">Switch persona</span>
          <select @change=${this._handleChange} .value=${this.currentPersona} aria-label="Switch persona">
            ${this.personas.map(p => html`
              <option value=${p.id} ?selected=${p.id === this.currentPersona}>${p.name}</option>
            `)}
          </select>
        </label>
      </div>
    `;
  }
}

customElements.define('persona-switcher', PersonaSwitcher);
