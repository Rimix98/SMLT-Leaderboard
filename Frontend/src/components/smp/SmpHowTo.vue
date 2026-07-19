<script setup lang="ts">
defineProps<{
  serverIp: string
}>()

const emit = defineEmits<{
  copyIp: []
}>()
</script>

<template>
  <section class="smp-section smp-start-section" id="start">
    <div class="smp-container">
      <div class="smp-section-heading smp-start-heading smp-reveal">
        <p class="smp-eyebrow">Всего несколько шагов</p>
        <h2>Как начать играть</h2>
        <p>Подключение займёт меньше минуты. Подойдёт Minecraft версии <strong>1.21</strong>.</p>
      </div>
      <ol class="smp-steps">
        <li class="smp-reveal"><span>1</span><p>Запустите Minecraft нужной версии.</p></li>
        <li class="smp-reveal"><span>2</span><p>Откройте раздел «Сетевая игра».</p></li>
        <li class="smp-reveal"><span>3</span><p>Нажмите «Добавить сервер».</p></li>
        <li class="smp-reveal"><span>4</span><p>Введите IP: <code>{{ serverIp }}</code>.</p></li>
        <li class="smp-reveal"><span>5</span><p>Подключитесь к серверу.</p></li>
      </ol>
      <div class="smp-join-card smp-reveal">
        <div>
          <span>Адрес сервера</span>
          <strong>{{ serverIp }}</strong>
        </div>
        <button class="smp-btn smp-btn-primary" type="button" @click="emit('copyIp')">Скопировать IP</button>
      </div>
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

.smp-start-section {
  background: linear-gradient(180deg, var(--color-primary), #0d130f);
}

.smp-start-heading { text-align: center; margin-inline: auto; }
.smp-start-heading .smp-eyebrow { justify-content: center; }

.smp-section-heading h2 {
  margin: 0 0 22px;
  font-size: clamp(2.35rem, 5vw, 4.25rem);
  line-height: 1.04;
  letter-spacing: -.05em;
}

.smp-section-heading > p:last-child {
  color: var(--color-text-secondary);
  font-size: 1.08rem;
}

.smp-steps {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  margin: 0;
  padding: 0;
  list-style: none;
}

.smp-steps li {
  position: relative;
  padding: 0 18px;
  text-align: center;
}

.smp-steps li:not(:last-child)::after {
  position: absolute;
  top: 24px;
  left: calc(50% + 34px);
  width: calc(100% - 68px);
  height: 1px;
  content: "";
  background: linear-gradient(90deg, #5cdb75, rgba(92,219,117,.12));
}

.smp-steps span {
  position: relative;
  z-index: 1;
  display: grid;
  width: 49px;
  height: 49px;
  margin: 0 auto 20px;
  place-items: center;
  color: #5cdb75;
  border: 1px solid rgba(92,219,117,.4);
  border-radius: 13px;
  background: var(--color-surface-2);
  font-weight: 850;
}

.smp-steps p { color: var(--color-text-secondary); font-size: .9rem; }
.smp-steps code { color: #5cdb75; font-family: ui-monospace, monospace; }

.smp-join-card {
  display: flex;
  max-width: 720px;
  align-items: center;
  justify-content: space-between;
  gap: 30px;
  margin: 70px auto 0;
  padding: 25px 28px;
  border: 1px solid rgba(92,219,117,.23);
  border-radius: 18px;
  background: linear-gradient(110deg, rgba(92,219,117,.11), rgba(255,255,255,.025));
}

.smp-join-card div { display: flex; flex-direction: column; }

.smp-join-card span {
  color: var(--color-text-muted);
  font-size: .72rem;
  letter-spacing: .08em;
  text-transform: uppercase;
}

.smp-join-card strong {
  margin-top: 2px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 1.35rem;
}

@media (max-width: 980px) {
  .smp-steps { grid-template-columns: repeat(3, 1fr); gap: 38px 0; }
  .smp-steps li:nth-child(3)::after { display: none; }
}

@media (max-width: 680px) {
  .smp-section { padding: 90px 0; }
  .smp-steps { display: flex; flex-direction: column; gap: 0; }
  .smp-steps li { display: grid; grid-template-columns: 48px 1fr; align-items: center; gap: 16px; padding: 13px 0; text-align: left; }
  .smp-steps li:not(:last-child)::after { top: 61px; left: 24px; display: block; width: 1px; height: 26px; background: linear-gradient(#5cdb75, transparent); }
  .smp-steps span { width: 46px; height: 46px; margin: 0; }
  .smp-join-card { align-items: stretch; flex-direction: column; margin-top: 45px; }
  .smp-join-card .smp-btn { width: 100%; }
  .smp-join-card strong { font-size: 1.15rem; }
}
</style>
