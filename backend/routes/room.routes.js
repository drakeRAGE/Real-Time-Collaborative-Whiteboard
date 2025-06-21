import express from 'express';

const router = express.Router();
import { deleteRoom } from '../controllers/room.controller.js';

router.delete('/:roomId', deleteRoom);

export default router; 