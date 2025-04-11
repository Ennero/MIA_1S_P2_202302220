// src/router/index.js
import { createRouter, createWebHistory } from 'vue-router';
// Importa los componentes que actuarán como "páginas"
// Tu vista principal actual (la consola)
import inicio from '@/views/StartPage.vue'; // <-- ¡NECESITARÁS MOVER TU LÓGICA DE App.vue AQUÍ!
// La nueva vista/ventana que quieres mostrar
import login from '@/views/LoginPage.vue'; // <-- ¡ESTE ES EL NUEVO ARCHIVO QUE CREARÁS!
import disk from '@/views/DiskPage.vue'; // <-- ¡ESTE ES EL NUEVO ARCHIVO QUE CREARÁS!

const routes = [
    {
        path: '/', // La ruta raíz mostrará la consola
        name: 'inicio',
        component: inicio
    },
    {
        path: '/login', // La URL para tu nueva ventana/vista
        name: 'login',
        component: login
        // Puedes añadir más rutas aquí
    },
    {
        path: '/disk',
        name: 'disk',
        component: disk
    },
];

const router = createRouter({
    history: createWebHistory(process.env.BASE_URL), // O createWebHashHistory()
    routes
});

export default router;