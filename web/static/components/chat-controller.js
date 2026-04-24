/**
 * Chat controller — wires up the send-message event from <chat-input>
 * to the backend API and appends responses to the message list.
 * Also handles persona-change events from <persona-switcher>.
 */
(function () {
  const page = document.querySelector('.chat-page');
  if (!page) return;

  const sessionId = page.dataset.sessionId;
  const messageList = page.querySelector('#message-list');
  const loader = page.querySelector('loading-indicator');
  const chatInput = page.querySelector('chat-input');
  const personaBadge = page.querySelector('.persona-badge');

  // --- Send message flow ---
  page.addEventListener('send-message', async (e) => {
    const { content } = e.detail;
    if (!content) return;

    // Append user message immediately
    appendMessage('user', content);
    scrollToBottom();

    // Show loading, disable input
    if (loader) loader.setAttribute('active', '');
    if (chatInput) chatInput.disabled = true;

    try {
      const res = await fetch(`/api/sessions/${sessionId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }));
        appendMessage('assistant', `Error: ${err.error || res.statusText}`);
        return;
      }

      const msg = await res.json();
      appendMessage(msg.role || 'assistant', msg.content || '', msg.persona_name || '');
    } catch (err) {
      appendMessage('assistant', `Network error: ${err.message}`);
    } finally {
      if (loader) loader.removeAttribute('active');
      if (chatInput) chatInput.disabled = false;
      scrollToBottom();
    }
  });

  // --- Persona switch flow ---
  page.addEventListener('persona-change', async (e) => {
    const { persona } = e.detail;
    if (!persona) return;

    try {
      const res = await fetch(`/api/sessions/${sessionId}/persona`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ persona }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }));
        appendMessage('system', `Error switching persona: ${err.error || res.statusText}`);
        return;
      }

      const data = await res.json();
      // Append system notification
      appendMessage('system', `Switched to ${data.display_name}`);
      // Update persona badge in header
      if (personaBadge) {
        personaBadge.textContent = data.display_name;
      }
      // Update the persona-switcher's current persona
      const switcher = page.querySelector('persona-switcher');
      if (switcher) {
        switcher.currentPersona = persona;
        switcher.dataset.currentPersona = persona;
      }
      scrollToBottom();
    } catch (err) {
      appendMessage('system', `Error switching persona: ${err.message}`);
      scrollToBottom();
    }
  });

  function appendMessage(role, content, personaName) {
    const el = document.createElement('chat-message');
    el.setAttribute('role', role);
    el.setAttribute('content', content);
    if (role === 'assistant' && personaName) {
      el.setAttribute('persona-name', personaName);
    }
    messageList.appendChild(el);
  }

  function scrollToBottom() {
    messageList.scrollTop = messageList.scrollHeight;
  }
})();
