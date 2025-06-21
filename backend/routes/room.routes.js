import express from 'express';
import { deleteRoom } from '../controllers/room.controller.js';

export const createRoomRouter = (io) => {
    const router = express.Router();

    router.delete('/rooms/:roomId', deleteRoom(io));

    return router;
};
