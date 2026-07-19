<script setup lang="ts">
defineProps<{
  serverIp: string
}>()

const emit = defineEmits<{
  copyIp: []
}>()

const SERVER_DOMAIN = 'smltsetup.duckdns.org'
</script>

<template>
  <section class="smp-section smp-start-section" id="start">
    <div class="smp-container">
      <div class="smp-section-heading smp-start-heading smp-reveal">
        <p class="smp-eyebrow">Всего несколько шагов до безумия</p>
        <h2>Как начать играть</h2>
        <p>Подключение займёт меньше минуты. Подойдёт Minecraft версии <strong>1.21.11</strong>.</p>
      </div>
      <ol class="smp-steps">
        <li class="smp-reveal"><span>1</span><p>Запустите Minecraft нужной версии.</p></li>
        <li class="smp-reveal"><span>2</span><p>Откройте раздел «Сетевая игра».</p></li>
        <li class="smp-reveal"><span>3</span><p>Нажмите «Добавить сервер».</p></li>
        <li class="smp-reveal"><span>4</span><p>Введите адрес: <code>{{ serverIp }}</code> или <code>{{ SERVER_DOMAIN }}</code>.</p></li>
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
