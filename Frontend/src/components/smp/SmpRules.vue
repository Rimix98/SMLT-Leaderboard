<template>
  <section class="smp-section smp-rules-section" id="rules">
    <div class="smp-container smp-rules-layout">
      <div class="smp-rules-intro smp-reveal">
        <p class="smp-eyebrow">Просто и по-честному</p>
        <h2>Правила сервера</h2>
        <p>Шесть простых принципов, которые помогают сохранить комфортную атмосферу для всех.</p>
      </div>
      <ol class="smp-rules-list">
        <li class="smp-reveal"><span>01</span><p>Уважайте других игроков.</p></li>
        <li class="smp-reveal"><span>02</span><p>Не используйте читы и запрещённые модификации.</p></li>
        <li class="smp-reveal"><span>03</span><p>Не ломайте чужие постройки.</p></li>
        <li class="smp-reveal"><span>04</span><p>Не создавайте лаг-машины.</p></li>
        <li class="smp-reveal"><span>05</span><p>Не используйте ошибки и уязвимости сервера.</p></li>
        <li class="smp-reveal"><span>06</span><p>Следуйте указаниям администрации.</p></li>
      </ol>
    </div>
  </section>
</template>

<style scoped>
.smp-section { padding: 128px 0; }

.smp-eyebrow {
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 0 0 22px;
  color: #5cdb75;
  font-size: .76rem;
  font-weight: 800;
  letter-spacing: .16em;
  text-transform: uppercase;
}

.smp-rules-layout {
  display: grid;
  grid-template-columns: .7fr 1.3fr;
  gap: 90px;
}

.smp-rules-intro { position: sticky; top: 125px; align-self: start; }

.smp-rules-intro h2 {
  margin: 0 0 22px;
  font-size: clamp(2.35rem, 5vw, 4.25rem);
  line-height: 1.04;
  letter-spacing: -.05em;
}

.smp-rules-intro > p { color: var(--color-text-secondary); font-size: 1.08rem; }

.smp-rules-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.smp-rules-list li {
  display: flex;
  align-items: center;
  gap: 24px;
  min-height: 88px;
  border-top: 1px solid var(--color-border);
}

.smp-rules-list li:last-child { border-bottom: 1px solid var(--color-border); }

.smp-rules-list span {
  color: #5cdb75;
  font-family: ui-monospace, monospace;
  font-size: .72rem;
}

.smp-rules-list p { margin: 0; font-size: 1.05rem; font-weight: 650; }

@media (max-width: 980px) {
  .smp-rules-layout { grid-template-columns: 1fr; gap: 55px; }
  .smp-rules-intro { position: static; max-width: 620px; }
}

@media (max-width: 680px) {
  .smp-section { padding: 90px 0; }
  .smp-rules-list li { align-items: flex-start; padding: 24px 0; }
  .smp-rules-list p { font-size: .98rem; }
}
</style>
